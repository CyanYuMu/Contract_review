package db

import (
	"context"
	"reflect"

	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/tool/su_time"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	src "cloud.google.com/go/firestore/apiv1/firestorepb"
)

type Firestore struct {
	client *firestore.Client
}

func NewFirestore(client *firestore.Client) *Firestore {
	return &Firestore{client: client}
}

func (f *Firestore) coll(table string) *firestore.CollectionRef {
	return f.client.Collection(table)
}

// Insert implements DB interface
func (f *Firestore) Insert(ctx context.Context, table string, row Row, options ...*InsertOptions) (string, error) {
	opt := dftInsertOptions()
	opt.Apply(options...)
	doc := f.coll(table).Doc(row.ID())
	var err error
	if opt.MergeAll {
		_, err = doc.Set(ctx, row, firestore.MergeAll)
	} else {
		_, err = doc.Set(ctx, row)
	}
	if err != nil {
		return "", err
	}
	if opt.ChangeHook != nil {
		opt.ChangeHook(ctx, ChangeRow{
			Table:  table,
			Id:     row.ID(),
			Action: ActionAdd,
		})
	}

	return doc.ID, nil
}

// Delete implements DB interface
func (f *Firestore) Delete(ctx context.Context, table string, id string, options ...*DeleteOptions) (int, error) {
	opt := dftDeleteOptions()
	opt.Apply(options...)
	_, err := f.coll(table).Doc(id).Delete(ctx)
	if err != nil {
		return 0, err
	}

	if opt.ChangeHook != nil {
		opt.ChangeHook(ctx, ChangeRow{
			Table:  table,
			Id:     id,
			Action: ActionDelete,
		})
	}

	return 1, nil
}

// Create implements DB interface
func (f *Firestore) Create(ctx context.Context, table string, row Row, options ...*CreateOptions) (*CreateResult, error) {
	opt := dftCreateOptions()
	opt.Apply(options...)
	doc := f.coll(table).Doc(row.ID())
	_, err := doc.Create(ctx, row)
	if err != nil {
		if status.Code(err) == codes.AlreadyExists {
			err = ErrDocAlreadyExists
		}
		return nil, err
	}
	if opt.ChangeHook != nil {
		opt.ChangeHook(ctx, ChangeRow{
			Table:  table,
			Id:     doc.ID,
			Action: ActionAdd,
		})
	}

	return &CreateResult{ID: doc.ID}, nil
}

// Update implements DB interface
func (f *Firestore) Update(ctx context.Context, table string, update UpdateOne, options ...*UpdateOptions) error {
	opt := dftUpdateOptions()
	opt.Apply(options...)
	_, err := f.coll(table).Doc(update.ID).Update(ctx, updatesToFirestoreFields(update.Updates))
	if err != nil {
		return err
	}
	if opt.ChangeHook != nil {
		opt.ChangeHook(ctx, ChangeRow{
			Table:  table,
			Id:     update.ID,
			Action: ActionUpdate,
			Data:   updatesToMapStringInterface(update.Updates),
		})
	}

	return nil
}

// Get implements DB interface
func (f *Firestore) Get(ctx context.Context, table string, id string, options ...*GetOptions) (*DocumentRef, error) {
	doc, err := f.coll(table).Doc(id).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, ErrDocNotFound
		}
		return nil, err
	}
	if !doc.Exists() {
		return nil, ErrDocNotFound
	}

	return &DocumentRef{
		ID:   doc.Ref.ID,
		Data: doc,
	}, nil
}

func (f *Firestore) Count(ctx context.Context, table string, conds Conds) (int64, error) {
	query := f.BuildQuery(table, conds)
	count := query.NewAggregationQuery().WithCount("total")
	get, err := count.Get(ctx)
	if err != nil {
		return 0, err
	}
	return get["total"].(*src.Value).GetIntegerValue(), nil
}

func (f *Firestore) BuildQuery(table string, conds Conds) firestore.Query {
	query := f.coll(table).Query

	for _, cond := range conds {
		op := "=="
		switch cond.Cond {
		case ">", ">=", "<", "<=", "not-in", "in", "!=", "array-contains":
			op = cond.Cond
		}
		query = query.Where(cond.Field, op, cond.Value)
	}
	return query
}

// Find implements DB interface
func (f *Firestore) Find(ctx context.Context, table string, conds Conds, options ...*FindOptions) (*Iterator, error) {
	query := f.BuildQuery(table, conds)

	if len(options) > 0 && options[0] != nil {
		opt := options[0]
		if opt.Limit > 0 {
			query = query.Limit(int(opt.Limit))
		}
		if opt.Offset > 0 {
			query = query.Offset(int(opt.Offset))
		}
		for _, sort := range opt.Sorts {
			dir := firestore.Asc
			if sort.Order == -1 {
				dir = firestore.Desc
			}
			query = query.OrderBy(sort.Field, dir)
		}
	}

	rs := &Iterator{}

	iter := query.Documents(ctx)
	defer iter.Stop()

	for {
		doc, err := iter.Next()
		if err != nil {
			if status.Code(err) == codes.NotFound {
				err = nil
			} else if err == iterator.Done {
				break
			}
			if err != nil {
				return nil, err
			}
		}
		if doc == nil {
			break
		}
		rs.items = append(rs.items, &DocumentRef{
			ID:   doc.Ref.ID,
			Data: doc,
		})
	}

	return rs, nil
}

// BatchCreate implements DB interface
func (f *Firestore) BatchCreate(ctx context.Context, table string, rows []Row, options ...*BatchWriteOptions) *BatchWriteResult {
	w := f.client.BulkWriter(ctx)
	rs := &BatchWriteResult{}
	opt := dftBatchWriteOptions()
	opt.Apply(options...)

	jobs := make([]*firestore.BulkWriterJob, 0, len(rows))

	for i := range rows {
		doc := f.coll(table).Doc(rows[i].ID())
		job, err := w.Create(doc, rows[i])
		if err != nil {
			rs.Errors = append(rs.Errors, err)
			return rs
		} else {
			jobs = append(jobs, job)
			rs.Ids = append(rs.Ids, doc.ID)
			rs.Affected++
		}

		if (i+1)%opt.BatchSize == 0 {
			w.Flush()
		}
	}

	w.End()

	for i := range jobs {
		_, err := jobs[i].Results()
		if err != nil {
			if IsAlreadyExists(err) && opt.ContinueOnExistsError == 1 {
				err = nil
			}
			rs.Errors = append(rs.Errors, err)
		} else {
			if opt.Hook() != nil {
				opt.Hook()(ctx, ChangeRow{
					Table:  table,
					Id:     rows[i].ID(),
					Action: ActionAdd,
				})
			}
		}
	}

	return rs
}

// BatchInsert implements DB interface
func (f *Firestore) BatchInsert(ctx context.Context, table string, rows []Row, options ...*BatchWriteOptions) *BatchWriteResult {
	w := f.client.BulkWriter(ctx)
	rs := &BatchWriteResult{}
	opt := dftBatchWriteOptions()
	opt.Apply(options...)

	jobs := make([]*firestore.BulkWriterJob, 0, len(rows))

	for i := range rows {
		doc := f.coll(table).Doc(rows[i].ID())
		job, err := w.Set(doc, rows[i])
		if err != nil {
			rs.Errors = append(rs.Errors, err)
			if opt.ContinueOnExistsError != 1 {
				break
			}
		} else {
			rs.Ids = append(rs.Ids, rows[i].ID())
			rs.Affected++
			jobs = append(jobs, job)
		}

		if (i+1)%opt.BatchSize == 0 {
			w.Flush()
		}
	}

	w.End()

	for i := range jobs {
		_, err := jobs[i].Results()
		if err != nil {
			rs.Errors = append(rs.Errors, err)
		} else {
			if opt.Hook() != nil {
				opt.Hook()(ctx, ChangeRow{
					Table:  table,
					Id:     rows[i].ID(),
					Action: ActionAdd,
				})
			}
		}
	}

	return rs
}

// BatchDelete implements DB interface
func (f *Firestore) BatchDelete(ctx context.Context, table string, ids []string, options ...*BatchWriteOptions) *BatchWriteResult {
	w := f.client.BulkWriter(ctx)
	rs := &BatchWriteResult{}

	opt := dftBatchWriteOptions()
	opt.Apply(options...)

	jobs := make([]*firestore.BulkWriterJob, 0, len(ids))

	for _, id := range ids {
		doc := f.coll(table).Doc(id)
		job, err := w.Delete(doc)
		if err != nil {
			rs.Errors = append(rs.Errors, err)
		} else {
			rs.Ids = append(rs.Ids, id)
			rs.Affected++
			jobs = append(jobs, job)
			if opt.Hook() != nil {
				opt.Hook()(ctx, ChangeRow{
					Table:  table,
					Id:     id,
					Action: ActionDelete,
				})
			}
		}
	}
	w.End()

	for i := range jobs {
		_, err := jobs[i].Results()
		if err != nil {
			rs.Errors = append(rs.Errors, err)
		}
	}

	return rs
}

func (f *Firestore) UpsertSingleField(ctx context.Context, table string, row UpsertSingleFields, options ...*UpsertSingleFieldRowOptions) (err error) {
	doc := f.coll(table).Doc(row.Id)
	var mergeAll = true
	if len(options) > 0 && options[0] != nil {
		mergeAll = !options[0].DisableMergeAll
	}
	ups := make(map[string]interface{})
	for key, val := range row.Fields {
		ups[key] = val
	}
	if mergeAll {
		_, err = doc.Set(ctx, ups, firestore.MergeAll)
	} else {
		_, err = doc.Set(ctx, ups)
	}

	return err
}

func (f *Firestore) Upsert(ctx context.Context, table string, row UpsertRow, options ...*UpsertOptions) (*UpsertRs, error) {
	doc := f.coll(table).Doc(row.Id)
	// 先尝试更新, 更是失败则进行插入
	opt := dftUpsertOptions()
	opt.Apply(options...)
	_, err := doc.Update(ctx, updatesToFirestoreFields(row.Updates))
	if err != nil {
		if status.Code(err) == codes.NotFound {
			inData := make([]Update, 0, len(row.Inserts)+len(row.Updates))
			inData = append(inData, row.Inserts...)
			inData = append(inData, row.Updates...)
			rowData := updatesToMapStringInterface(inData)
			_, err = doc.Set(ctx, rowData)
			if err != nil {
				return nil, err
			}

			// 调用Hook - 创建操作
			if opt.ChangeHook != nil {
				changeRow := ChangeRow{
					Table:  table,
					Id:     row.Id,
					Action: ActionAdd,
					Data:   rowData,
				}
				opt.ChangeHook(ctx, changeRow)
			}
		} else {
			return nil, err
		}
	} else {
		// 调用Hook - 更新操作
		if opt.ChangeHook != nil {
			updatedData := updatesToMapStringInterface(row.Updates)
			changeRow := ChangeRow{
				Table:  table,
				Id:     row.Id,
				Action: ActionUpdate,
				Data:   updatedData,
			}
			opt.ChangeHook(ctx, changeRow)
		}
	}

	return &UpsertRs{
		Id:         row.Id,
		MatchCount: 1,
	}, nil
}

// BatchUpdate implements DB interface
func (f *Firestore) BatchUpdate(ctx context.Context, table string, updates []UpdateOne, options ...*BatchWriteOptions) *BatchWriteResult {
	w := f.client.BulkWriter(ctx)
	opt := dftBatchWriteOptions()
	opt.Apply(options...)
	rs := &BatchWriteResult{}

	jobs := make([]*firestore.BulkWriterJob, 0, len(updates))
	hook := opt.ChangeHook

	for i := range updates {
		doc := f.coll(table).Doc(updates[i].ID)
		job, err := w.Update(doc, updatesToFirestoreFields(updates[i].Updates))
		if err != nil {
			rs.Errors = append(rs.Errors, err)
			if opt.ContinueOnError != 1 {
				return rs
			}
		} else {
			rs.Ids = append(rs.Ids, updates[i].ID)
			rs.Affected++
			jobs = append(jobs, job)
			if hook != nil {
				hook(ctx, ChangeRow{
					Table:  table,
					Id:     updates[i].ID,
					Action: ActionUpdate,
					Data:   updatesToMapStringInterface(updates[i].Updates),
				})
			}
		}

		if (i+1)%opt.BatchSize == 0 {
			w.Flush()
		}
	}

	w.End()

	for i := range jobs {
		_, err := jobs[i].Results()
		if err != nil {
			rs.Errors = append(rs.Errors, err)
		}
	}

	return rs
}

// BatchGet implements DB interface
func (f *Firestore) BatchGet(ctx context.Context, table string, ids []string, options ...*GetOptions) (*Iterator, error) {
	docs, err := f.client.GetAll(ctx, toDocRefs(f.coll(table), ids))
	if err != nil {
		return nil, err
	}

	rs := &Iterator{
		items: make([]*DocumentRef, 0, len(docs)),
	}

	for _, doc := range docs {
		if doc == nil {
			continue
		}
		if !doc.Exists() {
			continue
		}

		rs.items = append(rs.items, &DocumentRef{
			ID:   doc.Ref.ID,
			Data: doc,
		})
	}

	return rs, nil
}

func updatesToFirestoreFields(updates []Update) []firestore.Update {
	fields := make([]firestore.Update, len(updates))
	for i, update := range updates {
		var value interface{}
		if update.Op == "incr" {
			value = firestore.Increment(update.Value)
		} else {
			value = update.Value
		}
		//if update.Op == "addSet" {
		//	value = firestore.ArrayUnion(update.Value)
		//} else {
		//	value = update.Value
		//}
		fields[i] = firestore.Update{
			Path:  update.Field,
			Value: value,
		}
	}
	return fields
}

func updatesToMapStringInterface(updates []Update) map[string]interface{} {
	fields := make(map[string]interface{})
	for _, update := range updates {
		fields[update.Field] = update.Value
	}
	return fields
}

func toDocRefs(coll *firestore.CollectionRef, ids []string) []*firestore.DocumentRef {
	refs := make([]*firestore.DocumentRef, len(ids))
	for i, id := range ids {
		refs[i] = coll.Doc(id)
	}
	return refs
}

func (f *Firestore) ToUpdateOne(data any, options *UpdateOptions) (upData UpdateOne) {
	opt := dftFirestoreToUpdateOptions()
	opt.Apply(options)
	v := reflect.ValueOf(data)
	t := v.Type()
	if t.Kind() == reflect.Ptr {
		v = v.Elem()
		t = v.Type()
	}

	if t.Kind() != reflect.Struct {
		panic("data must be struct")
	}

	var containUpdateField bool
	processStruct(v, t, opt, &upData, &containUpdateField)

	if len(upData.Updates) > 0 {
		if !containUpdateField && !opt.IgnoreUpdateTimeField {
			upData.Updates = append(upData.Updates, Update{Field: UpdateField, Value: su_time.CurrentTimestampMilli()})
		}
	}

	return upData
}

func (f *Firestore) BatchUpsert(ctx context.Context, table string, rows []UpsertRow, opts ...*BatchWriteOptions) (*BatchUpsertRs, error) {
	opt := dftBatchWriteOptions()
	opt.Apply(opts...)
	rs := &BatchUpsertRs{
		InsertCount: 0,
		UpdateCount: 0,
	}

	if len(rows) == 0 {
		return rs, nil
	}

	// 第一阶段：尝试批量更新
	w := f.client.BulkWriter(ctx)
	updateJobs := make([]*firestore.BulkWriterJob, 0, len(rows))
	updateJobIndices := make([]int, 0, len(rows)) // 记录每个job对应的行索引

	// 先尝试批量更新
	for i := range rows {
		doc := f.coll(table).Doc(rows[i].Id)
		job, err := w.Update(doc, updatesToFirestoreFields(rows[i].Updates))
		if err != nil {
			su_logger.Error(ctx, err, "failed to create job for batch upsert update", su_logger.E().Any("row_id", rows[i].Id))
			if opt.ContinueOnError != 1 {
				return nil, err
			}
			continue
		}
		updateJobs = append(updateJobs, job)
		updateJobIndices = append(updateJobIndices, i)

		if (len(updateJobs))%opt.BatchSize == 0 {
			w.Flush()
		}
	}

	w.End()

	// 处理更新结果，收集需要插入的记录
	needInsertRows := make([]UpsertRow, 0)
	needInsertIndices := make([]int, 0)
	updateSuccessIndices := make([]int, 0)

	for i, job := range updateJobs {
		_, err := job.Results()
		if err != nil {
			if status.Code(err) != codes.NotFound {
				su_logger.Error(ctx, err, "failed to execute batch upsert update job", su_logger.E().Any("row_id", rows[updateJobIndices[i]].Id))
				if opt.ContinueOnError != 1 {
					return nil, err
				}
				continue
			} else {
				// 文档不存在，需要插入
				needInsertRows = append(needInsertRows, rows[updateJobIndices[i]])
				needInsertIndices = append(needInsertIndices, updateJobIndices[i])
			}
		} else {
			// 更新成功
			updateSuccessIndices = append(updateSuccessIndices, updateJobIndices[i])
			rs.UpdateCount++
		}
	}

	// 第二阶段：批量插入
	if len(needInsertRows) > 0 {
		insertW := f.client.BulkWriter(ctx)
		insertJobs := make([]*firestore.BulkWriterJob, 0, len(needInsertRows))

		for i := range needInsertRows {
			// 合并插入和更新数据
			inData := make([]Update, 0, len(needInsertRows[i].Inserts)+len(needInsertRows[i].Updates))
			inData = append(inData, needInsertRows[i].Inserts...)
			inData = append(inData, needInsertRows[i].Updates...)
			rowData := updatesToMapStringInterface(inData)

			// 使用 Set 操作进行批量插入
			doc := f.coll(table).Doc(needInsertRows[i].Id)
			job, err := insertW.Set(doc, rowData)
			if err != nil {
				su_logger.Error(ctx, err, "failed to create job for batch upsert insert", su_logger.E().Any("row_id", needInsertRows[i].Id))
				if opt.ContinueOnError != 1 {
					return nil, err
				}
				continue
			}
			insertJobs = append(insertJobs, job)

			if (len(insertJobs))%opt.BatchSize == 0 {
				insertW.Flush()
			}
		}

		insertW.End()

		// 处理插入结果
		for i, job := range insertJobs {
			_, err := job.Results()
			if err != nil {
				su_logger.Error(ctx, err, "failed to execute batch upsert insert job", su_logger.E().Any("row_id", needInsertRows[i].Id))
				if opt.ContinueOnError != 1 {
					return nil, err
				}
				continue
			}
			rs.InsertCount++
		}
	}

	// 调用 change hook
	if opt.ChangeHook != nil {
		// 处理更新成功的记录
		for _, idx := range updateSuccessIndices {
			opt.ChangeHook(ctx, ChangeRow{
				Table:  table,
				Id:     rows[idx].Id,
				Action: ActionUpdate,
				Data:   updatesToMapStringInterface(rows[idx].Updates),
			})
		}

		// 处理插入成功的记录
		for i, idx := range needInsertIndices {
			if i < len(needInsertRows) {
				inData := make([]Update, 0, len(needInsertRows[i].Inserts)+len(needInsertRows[i].Updates))
				inData = append(inData, needInsertRows[i].Inserts...)
				inData = append(inData, needInsertRows[i].Updates...)

				opt.ChangeHook(ctx, ChangeRow{
					Table:  table,
					Id:     rows[idx].Id,
					Action: ActionAdd,
					Data:   updatesToMapStringInterface(inData),
				})
			}
		}
	}

	return rs, nil
}
