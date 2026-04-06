package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gocql/gocql"
	jsoniter "github.com/json-iterator/go"
)

type ScyllaCli struct {
	*gocql.Session
}

type ScyllaConfig struct {
	Addresses []string
	// 租户空间. 类似于database
	Keyspace          string
	Username          string
	Password          string
	NumConns          *int
	NumRetries        *int
	Consistency       *gocql.Consistency
	SerialConsistency *gocql.SerialConsistency
	Compressor        *gocql.SnappyCompressor
}

func NewScyllaClient(ctx context.Context, config *ScyllaConfig) (*ScyllaCli, error) {
	cluster := gocql.NewCluster(config.Addresses...)
	cluster.Keyspace = config.Keyspace // 设置你的 Keyspace
	cluster.Authenticator = gocql.PasswordAuthenticator{
		Username: config.Username,
		Password: config.Password,
	}
	// 网络连接相关
	cluster.ProtoVersion = 4                   // 设置协议版本为 4，适用于大多数现代 ScyllaDB/Cassandra 集群
	cluster.ConnectTimeout = 10 * time.Second  // 缩短连接超时时间，避免长时间挂起
	cluster.Timeout = 30 * time.Second         // 查询超时时间，控制查询执行时的等待时间
	cluster.SocketKeepalive = 30 * time.Minute // 保持空闲连接活跃，避免连接过早关闭
	// 连接池配置
	cluster.NumConns = 10 // 每个节点的初始连接数，适合并发访问场景
	cluster.PoolConfig.HostSelectionPolicy = gocql.TokenAwareHostPolicy(
		gocql.RoundRobinHostPolicy()) // Token-Aware 策略结合 RoundRobin，确保负载均衡
	// 一致性和容错相关
	cluster.Consistency = gocql.Quorum            // Quorum 一致性级别，平衡性能和容错性
	cluster.SerialConsistency = gocql.LocalSerial // 轻量级事务使用 LocalSerial 保证局部一致性
	cluster.RetryPolicy = &gocql.ExponentialBackoffRetryPolicy{
		NumRetries: 3,
		Min:        100 * time.Millisecond,
		Max:        500 * time.Millisecond,
	}
	// 缓存与并发优化
	cluster.MaxPreparedStmts = 1000                        // 预处理语句缓存最大数量，适用于频繁使用的查询
	cluster.MaxRoutingKeyInfo = 1000                       // 查询信息缓存最大数量，优化路由键查询
	cluster.PageSize = 1000                                // 查询分页大小，避免单次查询返回过多数据
	cluster.WriteCoalesceWaitTime = 500 * time.Microsecond // 写入合并等待时间，减少写入系统调用
	// ScyllaDB 6.1 特有优化
	cluster.ReconnectInterval = 1 * time.Second       // 缩短重连间隔，快速处理节点失联问题
	cluster.MaxWaitSchemaAgreement = 30 * time.Second // 减少等待 schema 同步的时间

	//自定义参数
	if config.NumConns != nil {
		cluster.NumConns = *config.NumConns
	}
	if config.NumRetries != nil {
		cluster.RetryPolicy = &gocql.SimpleRetryPolicy{
			NumRetries: *config.NumRetries,
		}
	}
	if config.Consistency != nil {
		cluster.Consistency = *config.Consistency
	}
	if config.SerialConsistency != nil {
		cluster.SerialConsistency = *config.SerialConsistency
	}
	if config.Compressor != nil {
		cluster.Compressor = config.Compressor
	}
	session, err := cluster.CreateSession()
	if err != nil {
		return nil, err
	}
	return &ScyllaCli{session}, nil
}

type ScyllaDB struct {
	session *gocql.Session
}

func NewScyllaDB(session *gocql.Session) DB {
	inst := &ScyllaDB{session: session}

	return inst
}

func (s *ScyllaDB) UpsertSingleField(ctx context.Context, table string, row UpsertSingleFields, ops ...*UpsertSingleFieldRowOptions) (err error) {
	// todo
	return nil
}

// Insert implements DB interface
func (s *ScyllaDB) Insert(ctx context.Context, table string, row Row, options ...*InsertOptions) (string, error) {
	id := row.ID()

	data, err := jsoniter.Marshal(row)
	if err != nil {
		return "", err
	}

	query := fmt.Sprintf("INSERT INTO %s JSON ?", table)
	if err := s.session.Query(query, string(data)).WithContext(ctx).Exec(); err != nil {
		return "", err
	}

	return id, nil
}

// Delete implements DB interface
func (s *ScyllaDB) Delete(ctx context.Context, table string, id string, options ...*DeleteOptions) (int, error) {
	query := fmt.Sprintf("DELETE FROM %s WHERE id = ? IF EXISTS", table)
	var applied bool
	if err := s.session.Query(query, id).WithContext(ctx).Scan(&applied); err != nil {
		return 0, err
	}
	if applied {
		return 1, nil
	}
	return 0, nil
}

func (s *ScyllaDB) Upsert(ctx context.Context, table string, row UpsertRow, options ...*UpsertOptions) (*UpsertRs, error) {
	// 先尝试更新
	err := s.Update(ctx, table, UpdateOne{
		ID:      row.Id,
		Updates: row.Updates,
	})
	if err != nil {
		return nil, err
	}

	var inData = map[string]interface{}{}
	for _, update := range row.Updates {
		inData[update.Field] = update.Value
	}

	data, err := jsoniter.MarshalToString(inData)
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf("INSERT INTO %s JSON ?", table)
	var applied bool
	err = s.session.Query(query, data).WithContext(ctx).Exec()
	if err != nil {
		return nil, err
	}

	if !applied {
		return nil, ErrDocAlreadyExists
	}

	return &UpsertRs{
		Id: row.Id,
	}, nil
}

func (s *ScyllaDB) Count(ctx context.Context, table string, conds Conds) (int64, error) {
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", table)
	var count int64
	if err := s.session.Query(query).WithContext(ctx).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// Create implements DB interface
func (s *ScyllaDB) Create(ctx context.Context, table string, row Row, options ...*CreateOptions) (*CreateResult, error) {
	id := row.ID()
	data, err := jsoniter.MarshalToString(row)
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf("INSERT INTO %s JSON ?", table)
	var applied bool
	err = s.session.Query(query, data).WithContext(ctx).Exec()
	if err != nil {
		return nil, err
	}

	if !applied {
		return nil, ErrDocAlreadyExists
	}

	return &CreateResult{ID: id}, nil
}

// Update implements DB interface
func (s *ScyllaDB) Update(ctx context.Context, table string, update UpdateOne, options ...*UpdateOptions) error {
	sets := make([]string, 0, len(update.Updates))
	args := make([]interface{}, 0, len(update.Updates)+1)

	for _, u := range update.Updates {
		switch u.Op {
		case "incr":
			sets = append(sets, fmt.Sprintf("%s = %s + ?", u.Field, u.Field))
			args = append(args, u.Value)
		default:
			sets = append(sets, fmt.Sprintf("%s = ?", u.Field))
			args = append(args, u.Value)
		}
	}

	args = append(args, update.ID)

	query := fmt.Sprintf("UPDATE %s SET %s WHERE id = ?",
		table, strings.Join(sets, ", "))

	if err := s.session.Query(query, args...).WithContext(ctx).Exec(); err != nil {
		return err
	}

	return nil
}

// Get implements DB interface
func (s *ScyllaDB) Get(ctx context.Context, table string, id string, options ...*GetOptions) (*DocumentRef, error) {
	var err error
	var query string
	if len(options) > 0 && options[0] != nil && len(options[0].Fields) > 0 {
		fields := strings.Join(options[0].Fields, ", ")
		query = fmt.Sprintf("SELECT %s FROM %s WHERE id = ? LIMIT 1", fields, table)
	} else {
		query = fmt.Sprintf("SELECT * FROM %s WHERE id = ? LIMIT 1", table)
	}

	ref := &DocumentRef{
		ID:   id,
		Data: nil,
	}

	iter := s.session.Query(query, id).WithContext(ctx).Iter()
	defer iter.Close()
	m, err := iter.SliceMap()
	if err != nil {
		return nil, err
	}
	if len(m) == 0 {
		return nil, ErrDocNotFound
	}
	row := m[0]

	ref.Data, err = jsoniter.Marshal(row)
	if err != nil {
		return nil, err
	}

	return ref, nil
}

// Find implements DB interface
func (s *ScyllaDB) Find(ctx context.Context, table string, conds Conds, options ...*FindOptions) (*Iterator, error) {
	whereClause, args := buildScyllaWhereClause(conds)

	// 构建基础查询
	var query string
	if len(options) > 0 && options[0] != nil && len(options[0].Fields) > 0 {
		fields := strings.Join(options[0].Fields, ", ")
		query = fmt.Sprintf("SELECT %s FROM %s %s", fields, table, whereClause)
	} else {
		query = fmt.Sprintf("SELECT * FROM %s %s", table, whereClause)
	}

	// 添加排序
	if len(options) > 0 && options[0] != nil && len(options[0].Sorts) > 0 {
		query += " ORDER BY " + buildScyllaOrderByClause(options[0].Sorts)
	}

	// 添加分页
	if len(options) > 0 && options[0] != nil {
		if options[0].Limit > 0 {
			query += fmt.Sprintf(" LIMIT %d", options[0].Limit)
		}
		if options[0].Offset > 0 {
			// ScyllaDB 不直接支持 OFFSET，需要使用 token 实现
			// 这里是简化实现，实际应用中可能需要更复杂的分页逻辑
			query += fmt.Sprintf(" OFFSET %d", options[0].Offset)
		}
	}

	iter := s.session.Query(query, args...).WithContext(ctx).Iter()
	defer iter.Close()

	var results []*DocumentRef
	rows, err := iter.SliceMap()
	if err != nil {
		return nil, err
	}

	for _, row := range rows {
		id, _ := row["id"].(string)
		jsonData, err := jsoniter.Marshal(row)
		if err != nil {
			return nil, err
		}

		results = append(results, &DocumentRef{
			ID:   id,
			Data: jsonData,
		})
	}

	return &Iterator{items: results}, nil
}

// BatchCreate implements DB interface
func (s *ScyllaDB) BatchCreate(ctx context.Context, table string, rows []Row, options ...*BatchWriteOptions) *BatchWriteResult {
	opt := dftBatchWriteOptions()
	if len(options) > 0 {
		opt.Apply(options[0])
	}

	result := &BatchWriteResult{
		Ids:    make([]string, 0, len(rows)),
		Errors: make([]error, 0),
	}

	batch := s.session.NewBatch(gocql.LoggedBatch).WithContext(ctx)
	for _, row := range rows {
		id := row.ID()
		if id == "" {
			id = gocql.TimeUUID().String()
			row.SetID(id)
		}

		data, err := jsoniter.Marshal(row)
		if err != nil {
			result.Errors = append(result.Errors, err)
			if opt.ContinueOnError != 1 {
				return result
			}
			continue
		}

		batch.Query(fmt.Sprintf("INSERT INTO %s JSON ? IF NOT EXISTS", table), string(data))
		result.Ids = append(result.Ids, id)
	}

	if err := s.session.ExecuteBatch(batch); err != nil {
		result.Errors = append(result.Errors, err)
		return result
	}

	result.Affected = int64(len(result.Ids))
	return result
}

// BatchUpdate implements DB interface
func (s *ScyllaDB) BatchUpdate(ctx context.Context, table string, updates []UpdateOne, options ...*BatchWriteOptions) *BatchWriteResult {
	opt := dftBatchWriteOptions()
	if len(options) > 0 {
		opt.Apply(options[0])
	}

	result := &BatchWriteResult{
		Errors: make([]error, 0),
	}

	// Execute updates one by one
	for _, update := range updates {
		sets := make([]string, len(update.Updates))
		args := make([]interface{}, len(update.Updates)+1)

		for i, u := range update.Updates {
			if u.Op == "incr" {
				sets[i] = fmt.Sprintf("%s = %s + ?", u.Field, u.Field)
			} else {
				sets[i] = fmt.Sprintf("%s = ?", u.Field)
			}
			args[i] = u.Value
		}
		args[len(args)-1] = update.ID

		// Remove IF EXISTS since LWT is not supported
		query := fmt.Sprintf("UPDATE %s SET %s WHERE id = ?", table, strings.Join(sets, ", "))

		if err := s.session.Query(query, args...).WithContext(ctx).Exec(); err != nil {
			result.Errors = append(result.Errors, err)
			if opt.ContinueOnError != 1 {
				return result
			}
			continue
		}
		result.Affected++
	}

	return result
}

// BatchDelete implements DB interface
func (s *ScyllaDB) BatchDelete(ctx context.Context, table string, ids []string, options ...*BatchWriteOptions) *BatchWriteResult {
	opt := dftBatchWriteOptions()
	if len(options) > 0 {
		opt.Apply(options[0])
	}

	result := &BatchWriteResult{
		Errors: make([]error, 0),
	}

	if len(ids) == 0 {
		return result
	}

	// Use IN clause for batch deletion
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1]
	query := fmt.Sprintf("DELETE FROM %s WHERE id IN (%s)", table, placeholders)

	args := make([]interface{}, len(ids))
	for i, id := range ids {
		args[i] = id
	}

	if err := s.session.Query(query, args...).WithContext(ctx).Exec(); err != nil {
		result.Errors = append(result.Errors, err)
		return result
	}

	result.Affected = int64(len(ids))
	return result
}

// BatchGet implements DB interface
func (s *ScyllaDB) BatchGet(ctx context.Context, table string, ids []string, options ...*GetOptions) (*Iterator, error) {
	fields := "*"
	if len(options) > 0 && options[0] != nil && len(options[0].Fields) > 0 {
		fields = strings.Join(options[0].Fields, ", ")
	}

	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1]
	query := fmt.Sprintf("SELECT %s FROM %s WHERE id IN (%s)", fields, table, placeholders)

	args := make([]interface{}, len(ids))
	for i, id := range ids {
		args[i] = id
	}

	iter := s.session.Query(query, args...).WithContext(ctx).Iter()
	defer iter.Close()

	// var results []*DocumentRef
	rs := &Iterator{
		items: make([]*DocumentRef, 0, len(ids)),
	}
	sliceData, err := iter.SliceMap()
	if err != nil {
		return nil, err
	}
	for i := range sliceData {
		byteData, _ := jsoniter.Marshal(sliceData[i])
		rs.items = append(rs.items, &DocumentRef{
			ID:   sliceData[i]["id"].(string),
			Data: byteData,
		})
	}

	return rs, nil
}

// BatchInsert implements DB interface
func (s *ScyllaDB) BatchInsert(ctx context.Context, table string, rows []Row, options ...*BatchWriteOptions) *BatchWriteResult {
	opt := dftBatchWriteOptions()
	if len(options) > 0 {
		opt.Apply(options[0])
	}

	result := &BatchWriteResult{
		Ids:    make([]string, 0, len(rows)),
		Errors: make([]error, 0),
	}

	batch := s.session.NewBatch(gocql.LoggedBatch).WithContext(ctx)
	for _, row := range rows {
		id := row.ID()
		if id == "" {
			id = gocql.TimeUUID().String()
			row.SetID(id)
		}

		data, err := jsoniter.Marshal(row)
		if err != nil {
			result.Errors = append(result.Errors, err)
			if opt.ContinueOnError != 1 {
				return result
			}
			continue
		}

		// 使用 UPSERT 语义
		query := fmt.Sprintf("INSERT INTO %s JSON ?", table)
		batch.Query(query, string(data))
		result.Ids = append(result.Ids, id)
	}

	if err := s.session.ExecuteBatch(batch); err != nil {
		result.Errors = append(result.Errors, err)
		return result
	}

	result.Affected = int64(len(result.Ids))
	return result
}

// Helper functions
func buildScyllaWhereClause(conds Conds) (string, []interface{}) {
	if len(conds) == 0 {
		return "", nil
	}

	clauses := make([]string, 0, len(conds))
	args := make([]interface{}, 0, len(conds))

	for _, cond := range conds {
		switch strings.ToLower(cond.Cond) {
		case "in":
			clauses = append(clauses, fmt.Sprintf("%s IN ?", cond.Field))
			args = append(args, cond.Value)
		case ">", ">=", "<", "<=":
			clauses = append(clauses, fmt.Sprintf("%s %s ?", cond.Field, cond.Cond))
			args = append(args, cond.Value)
		case "==", "=":
			clauses = append(clauses, fmt.Sprintf("%s = ?", cond.Field))
			args = append(args, cond.Value)
		default:
			clauses = append(clauses, fmt.Sprintf("%s = ?", cond.Field))
			args = append(args, cond.Value)
		}
	}

	return "WHERE " + strings.Join(clauses, " AND "), args
}

func buildScyllaOrderByClause(sorts Sorts) string {
	if len(sorts) == 0 {
		return ""
	}

	orders := make([]string, len(sorts))
	for i, sort := range sorts {
		direction := "ASC"
		if sort.Order == -1 {
			direction = "DESC"
		}
		orders[i] = fmt.Sprintf("%s %s", sort.Field, direction)
	}

	return strings.Join(orders, ", ")
}

func (s *ScyllaDB) ToUpdateOne(data any, options *UpdateOptions) (upData UpdateOne) {
	// todo
	return upData
}

// BatchUpsert implements DB interface
func (s *ScyllaDB) BatchUpsert(ctx context.Context, table string, rows []UpsertRow, opts ...*BatchWriteOptions) (*BatchUpsertRs, error) {
	opt := dftBatchWriteOptions()
	opt.Apply(opts...)

	rs := &BatchUpsertRs{
		InsertCount: 0,
		UpdateCount: 0,
	}

	if len(rows) == 0 {
		return rs, nil
	}

	// ScyllaDB 的批量 upsert 实现
	// 由于 ScyllaDB 的 CQL 语法限制，我们需要分别处理更新和插入
	for _, row := range rows {
		// 先尝试更新
		err := s.Update(ctx, table, UpdateOne{
			ID:      row.Id,
			Updates: row.Updates,
		})

		if err != nil {
			// 更新失败，尝试插入
			inData := make(map[string]interface{})

			// 合并插入和更新字段
			for _, update := range row.Inserts {
				inData[update.Field] = update.Value
			}
			for _, update := range row.Updates {
				inData[update.Field] = update.Value
			}

			// 添加 ID
			inData["id"] = row.Id

			data, err := jsoniter.MarshalToString(inData)
			if err != nil {
				if opt.ContinueOnError != 1 {
					return nil, err
				}
				continue
			}

			query := fmt.Sprintf("INSERT INTO %s JSON ?", table)
			if err := s.session.Query(query, data).WithContext(ctx).Exec(); err != nil {
				if opt.ContinueOnError != 1 {
					return nil, err
				}
				continue
			}

			rs.InsertCount++

			// 调用 change hook
			if opt.ChangeHook != nil {
				opt.ChangeHook(ctx, ChangeRow{
					Table:  table,
					Id:     row.Id,
					Action: ActionAdd,
					Data:   inData,
				})
			}
		} else {
			// 更新成功
			rs.UpdateCount++

			// 调用 change hook
			if opt.ChangeHook != nil {
				updatedData := updatesToMapStringInterface(row.Updates)
				opt.ChangeHook(ctx, ChangeRow{
					Table:  table,
					Id:     row.Id,
					Action: ActionUpdate,
					Data:   updatedData,
				})
			}
		}
	}

	return rs, nil
}
