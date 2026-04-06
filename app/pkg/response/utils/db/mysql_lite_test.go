package db

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const (
	testDSN   = "dw_meta_user_develop:WEHs!74637shsonnbjw%ZdL6VsK$@tcp(rm-rj94vu3571lhp26wero.mysql.rds-aliyun-america.rds.aliyuncs.com:3306)/dw_meta_develop?charset=utf8mb4&parseTime=True&loc=Local"
	testTable = "sqllite_test"
)

// TestRow 测试用的 Row 实现
type TestRow struct {
	Id        string `json:"id"`
	Name      string `json:"name"`
	Age       int    `json:"age"`
	Score     int    `json:"score"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"update_at"`
}

func (t *TestRow) ID() string {
	return t.Id
}

func (t *TestRow) SetID(id string) {
	t.Id = id
}

// setupTestDB 创建测试数据库连接和表
func setupTestDB(t *testing.T) *MysqlLite {
	db, err := sql.Open("mysql", testDSN)
	if err != nil {
		t.Fatalf("failed to open mysql: %v", err)
	}

	if err := db.Ping(); err != nil {
		t.Fatalf("failed to ping mysql: %v", err)
	}

	// 创建测试表
	createTableSQL := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id VARCHAR(64) PRIMARY KEY,
			name VARCHAR(128) DEFAULT '',
			age INT DEFAULT 0,
			score INT DEFAULT 0,
			created_at BIGINT DEFAULT 0,
			update_at BIGINT DEFAULT 0
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
	`, testTable)

	_, err = db.Exec(createTableSQL)
	if err != nil {
		t.Fatalf("failed to create test table: %v", err)
	}

	// 注释掉清空表的操作，以便保留测试数据进行观察
	// _, err = db.Exec(fmt.Sprintf("DELETE FROM %s WHERE 1=1", testTable))
	// if err != nil {
	// 	t.Fatalf("failed to clean test table: %v", err)
	// }

	return NewMysqlLiteFromDB(db)
}

// cleanupTestDB 清理测试数据
func cleanupTestDB(t *testing.T, lite *MysqlLite) {
	// 注释掉以便测试后查看数据
	// _, err := lite.DB().Exec(fmt.Sprintf("DELETE FROM %s WHERE 1=1", testTable))
	// if err != nil {
	// 	t.Logf("failed to clean test table: %v", err)
	// }
}

// TestMysqlLite_Create 测试创建记录
func TestMysqlLite_Create(t *testing.T) {
	lite := setupTestDB(t)
	defer cleanupTestDB(t, lite)

	ctx := context.Background()

	// 测试创建
	row := &TestRow{
		Id:        "test_create_1",
		Name:      "Alice",
		Age:       25,
		Score:     100,
		CreatedAt: time.Now().UnixMilli(),
	}

	result, err := lite.Create(ctx, testTable, row)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if result.ID != row.Id {
		t.Errorf("expected ID %s, got %s", row.Id, result.ID)
	}

	// 测试重复创建应该报错
	_, err = lite.Create(ctx, testTable, row)
	if err != ErrDocAlreadyExists {
		t.Errorf("expected ErrDocAlreadyExists, got %v", err)
	}

	t.Log("TestMysqlLite_Create passed")
}

// TestMysqlLite_Insert 测试插入（upsert）
func TestMysqlLite_Insert(t *testing.T) {
	lite := setupTestDB(t)
	defer cleanupTestDB(t, lite)

	ctx := context.Background()

	row := &TestRow{
		Id:   "test_insert_1",
		Name: "Bob",
		Age:  30,
	}

	// 第一次插入
	id, err := lite.Insert(ctx, testTable, row)
	if err != nil {
		t.Fatalf("Insert failed: %v", err)
	}
	if id != row.Id {
		t.Errorf("expected ID %s, got %s", row.Id, id)
	}

	// 第二次插入（更新）
	row.Name = "Bob Updated"
	row.Age = 31
	_, err = lite.Insert(ctx, testTable, row)
	if err != nil {
		t.Fatalf("Insert (update) failed: %v", err)
	}

	// 验证更新
	ref, err := lite.Get(ctx, testTable, row.Id)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	data := ref.Data.(map[string]interface{})
	if data["name"] != "Bob Updated" {
		t.Errorf("expected name 'Bob Updated', got %v", data["name"])
	}

	t.Log("TestMysqlLite_Insert passed")
}

// TestMysqlLite_Get 测试获取记录
func TestMysqlLite_Get(t *testing.T) {
	lite := setupTestDB(t)
	defer cleanupTestDB(t, lite)

	ctx := context.Background()

	// 插入测试数据
	row := &TestRow{
		Id:   "test_get_1",
		Name: "Charlie",
		Age:  28,
	}
	_, err := lite.Create(ctx, testTable, row)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// 测试获取
	ref, err := lite.Get(ctx, testTable, row.Id)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if ref.ID != row.Id {
		t.Errorf("expected ID %s, got %s", row.Id, ref.ID)
	}

	// 测试获取不存在的记录
	_, err = lite.Get(ctx, testTable, "non_existent_id")
	if err != ErrDocNotFound {
		t.Errorf("expected ErrDocNotFound, got %v", err)
	}

	// 测试指定字段获取
	_, err = lite.Get(ctx, testTable, row.Id, &GetOptions{Fields: []string{"id", "name"}})
	if err != nil {
		t.Fatalf("Get with fields failed: %v", err)
	}

	t.Log("TestMysqlLite_Get passed")
}

// TestMysqlLite_Update 测试更新记录
func TestMysqlLite_Update(t *testing.T) {
	lite := setupTestDB(t)
	defer cleanupTestDB(t, lite)

	ctx := context.Background()

	// 插入测试数据
	row := &TestRow{
		Id:    "test_update_1",
		Name:  "David",
		Age:   35,
		Score: 50,
	}
	_, err := lite.Create(ctx, testTable, row)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// 测试普通更新
	err = lite.Update(ctx, testTable, UpdateOne{
		ID: row.Id,
		Updates: []Update{
			{Field: "name", Value: "David Updated"},
			{Field: "age", Value: 36},
		},
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// 验证更新
	ref, err := lite.Get(ctx, testTable, row.Id)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	data := ref.Data.(map[string]interface{})
	if data["name"] != "David Updated" {
		t.Errorf("expected name 'David Updated', got %v", data["name"])
	}

	// 测试 incr 更新
	err = lite.Update(ctx, testTable, UpdateOne{
		ID: row.Id,
		Updates: []Update{
			{Field: "score", Op: "incr", Value: 10},
		},
	})
	if err != nil {
		t.Fatalf("Update incr failed: %v", err)
	}

	ref, err = lite.Get(ctx, testTable, row.Id)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	data = ref.Data.(map[string]interface{})
	score := int(data["score"].(int64))
	if score != 60 {
		t.Errorf("expected score 60, got %v", score)
	}

	t.Log("TestMysqlLite_Update passed")
}

// TestMysqlLite_Delete 测试删除记录
func TestMysqlLite_Delete(t *testing.T) {
	lite := setupTestDB(t)
	defer cleanupTestDB(t, lite)

	ctx := context.Background()

	// 插入测试数据
	row := &TestRow{
		Id:   "test_delete_1",
		Name: "Eve",
	}
	_, err := lite.Create(ctx, testTable, row)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// 测试删除
	affected, err := lite.Delete(ctx, testTable, row.Id)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if affected != 1 {
		t.Errorf("expected 1 affected, got %d", affected)
	}

	// 验证删除
	_, err = lite.Get(ctx, testTable, row.Id)
	if err != ErrDocNotFound {
		t.Errorf("expected ErrDocNotFound, got %v", err)
	}

	t.Log("TestMysqlLite_Delete passed")
}

// TestMysqlLite_Find 测试查询记录
func TestMysqlLite_Find(t *testing.T) {
	lite := setupTestDB(t)
	defer cleanupTestDB(t, lite)

	ctx := context.Background()

	// 插入多条测试数据
	for i := 1; i <= 5; i++ {
		row := &TestRow{
			Id:   fmt.Sprintf("test_find_%d", i),
			Name: fmt.Sprintf("User%d", i),
			Age:  20 + i,
		}
		_, err := lite.Create(ctx, testTable, row)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	// 测试查询所有
	iter, err := lite.Find(ctx, testTable, nil)
	if err != nil {
		t.Fatalf("Find failed: %v", err)
	}
	if iter.IsEmpty() {
		t.Error("expected non-empty iterator")
	}

	// 测试条件查询
	iter, err = lite.Find(ctx, testTable, Conds{
		{Field: "age", Cond: ">", Value: 22},
	})
	if err != nil {
		t.Fatalf("Find with conds failed: %v", err)
	}

	count := 0
	for {
		_, err := iter.Next()
		if err != nil {
			break
		}
		count++
	}
	if count != 3 {
		t.Errorf("expected 3 records, got %d", count)
	}

	// 测试排序和分页
	_, err = lite.Find(ctx, testTable, nil, &FindOptions{
		Sorts:  Sorts{{Field: "age", Order: -1}},
		Limit:  2,
		Offset: 1,
	})
	if err != nil {
		t.Fatalf("Find with options failed: %v", err)
	}

	// 测试 IN 查询
	_, err = lite.Find(ctx, testTable, Conds{
		{Field: "id", Cond: "in", Value: []string{"test_find_1", "test_find_2"}},
	})
	if err != nil {
		t.Fatalf("Find with IN failed: %v", err)
	}

	t.Log("TestMysqlLite_Find passed")
}

// TestMysqlLite_Count 测试计数
func TestMysqlLite_Count(t *testing.T) {
	lite := setupTestDB(t)
	defer cleanupTestDB(t, lite)

	ctx := context.Background()

	// 插入测试数据
	for i := 1; i <= 3; i++ {
		row := &TestRow{
			Id:  fmt.Sprintf("test_count_%d", i),
			Age: 20 + i,
		}
		_, _ = lite.Create(ctx, testTable, row)
	}

	// 测试总数
	count, err := lite.Count(ctx, testTable, nil)
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 3 {
		t.Errorf("expected count 3, got %d", count)
	}

	// 测试条件计数
	count, err = lite.Count(ctx, testTable, Conds{
		{Field: "age", Cond: ">=", Value: 22},
	})
	if err != nil {
		t.Fatalf("Count with conds failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected count 2, got %d", count)
	}

	t.Log("TestMysqlLite_Count passed")
}

// TestMysqlLite_BatchCreate 测试批量创建
func TestMysqlLite_BatchCreate(t *testing.T) {
	lite := setupTestDB(t)
	defer cleanupTestDB(t, lite)

	ctx := context.Background()

	rows := []Row{
		&TestRow{Id: "batch_create_1", Name: "User1"},
		&TestRow{Id: "batch_create_2", Name: "User2"},
		&TestRow{Id: "batch_create_3", Name: "User3"},
	}

	result := lite.BatchCreate(ctx, testTable, rows)
	if result.Error() != nil {
		t.Fatalf("BatchCreate failed: %v", result.Error())
	}
	if result.Affected != 3 {
		t.Errorf("expected 3 affected, got %d", result.Affected)
	}

	// 验证
	count, _ := lite.Count(ctx, testTable, nil)
	if count != 3 {
		t.Errorf("expected count 3, got %d", count)
	}

	t.Log("TestMysqlLite_BatchCreate passed")
}

// TestMysqlLite_BatchInsert 测试批量插入
func TestMysqlLite_BatchInsert(t *testing.T) {
	lite := setupTestDB(t)
	defer cleanupTestDB(t, lite)

	ctx := context.Background()

	rows := []Row{
		&TestRow{Id: "batch_insert_1", Name: "User1", Age: 20},
		&TestRow{Id: "batch_insert_2", Name: "User2", Age: 21},
	}

	result := lite.BatchInsert(ctx, testTable, rows)
	if result.Error() != nil {
		t.Fatalf("BatchInsert failed: %v", result.Error())
	}

	// 再次插入（更新）
	rows = []Row{
		&TestRow{Id: "batch_insert_1", Name: "User1 Updated", Age: 25},
		&TestRow{Id: "batch_insert_3", Name: "User3", Age: 22},
	}

	result = lite.BatchInsert(ctx, testTable, rows)
	if result.Error() != nil {
		t.Fatalf("BatchInsert (update) failed: %v", result.Error())
	}

	// 验证
	count, _ := lite.Count(ctx, testTable, nil)
	if count != 3 {
		t.Errorf("expected count 3, got %d", count)
	}

	t.Log("TestMysqlLite_BatchInsert passed")
}

// TestMysqlLite_BatchDelete 测试批量删除
func TestMysqlLite_BatchDelete(t *testing.T) {
	lite := setupTestDB(t)
	defer cleanupTestDB(t, lite)

	ctx := context.Background()

	// 插入测试数据
	for i := 1; i <= 5; i++ {
		_, _ = lite.Create(ctx, testTable, &TestRow{Id: fmt.Sprintf("batch_del_%d", i)})
	}

	// 批量删除
	result := lite.BatchDelete(ctx, testTable, []string{"batch_del_1", "batch_del_2", "batch_del_3"})
	if result.Error() != nil {
		t.Fatalf("BatchDelete failed: %v", result.Error())
	}
	if result.Affected != 3 {
		t.Errorf("expected 3 affected, got %d", result.Affected)
	}

	// 验证
	count, _ := lite.Count(ctx, testTable, nil)
	if count != 2 {
		t.Errorf("expected count 2, got %d", count)
	}

	t.Log("TestMysqlLite_BatchDelete passed")
}

// TestMysqlLite_BatchUpdate 测试批量更新（CASE WHEN）
func TestMysqlLite_BatchUpdate(t *testing.T) {
	lite := setupTestDB(t)
	defer cleanupTestDB(t, lite)

	ctx := context.Background()

	// 插入测试数据
	for i := 1; i <= 3; i++ {
		_, _ = lite.Create(ctx, testTable, &TestRow{
			Id:    fmt.Sprintf("batch_update_%d", i),
			Name:  fmt.Sprintf("User%d", i),
			Score: 50,
		})
	}

	// 批量更新（使用 CASE WHEN）
	updates := []UpdateOne{
		{
			ID: "batch_update_1",
			Updates: []Update{
				{Field: "name", Value: "Updated1"},
				{Field: "score", Op: "incr", Value: 10},
			},
		},
		{
			ID: "batch_update_2",
			Updates: []Update{
				{Field: "name", Value: "Updated2"},
				{Field: "score", Op: "incr", Value: 20},
			},
		},
		{
			ID: "batch_update_3",
			Updates: []Update{
				{Field: "name", Value: "Updated3"},
				{Field: "score", Op: "incr", Value: 30},
			},
		},
	}

	result := lite.BatchUpdate(ctx, testTable, updates)
	if result.Error() != nil {
		t.Fatalf("BatchUpdate failed: %v", result.Error())
	}

	// 验证更新结果
	ref, _ := lite.Get(ctx, testTable, "batch_update_1")
	data := ref.Data.(map[string]interface{})
	if data["name"] != "Updated1" {
		t.Errorf("expected name 'Updated1', got %v", data["name"])
	}
	if int(data["score"].(int64)) != 60 {
		t.Errorf("expected score 60, got %v", data["score"])
	}

	ref, _ = lite.Get(ctx, testTable, "batch_update_3")
	data = ref.Data.(map[string]interface{})
	if int(data["score"].(int64)) != 80 {
		t.Errorf("expected score 80, got %v", data["score"])
	}

	t.Log("TestMysqlLite_BatchUpdate passed")
}

// TestMysqlLite_BatchGet 测试批量获取
func TestMysqlLite_BatchGet(t *testing.T) {
	lite := setupTestDB(t)
	defer cleanupTestDB(t, lite)

	ctx := context.Background()

	// 插入测试数据
	for i := 1; i <= 3; i++ {
		_, _ = lite.Create(ctx, testTable, &TestRow{
			Id:   fmt.Sprintf("batch_get_%d", i),
			Name: fmt.Sprintf("User%d", i),
		})
	}

	// 批量获取
	iter, err := lite.BatchGet(ctx, testTable, []string{"batch_get_1", "batch_get_2", "batch_get_3"})
	if err != nil {
		t.Fatalf("BatchGet failed: %v", err)
	}

	count := 0
	for {
		_, err := iter.Next()
		if err != nil {
			break
		}
		count++
	}
	if count != 3 {
		t.Errorf("expected 3 records, got %d", count)
	}

	t.Log("TestMysqlLite_BatchGet passed")
}

// TestMysqlLite_Upsert 测试更新或插入
func TestMysqlLite_Upsert(t *testing.T) {
	lite := setupTestDB(t)
	defer cleanupTestDB(t, lite)

	ctx := context.Background()

	// 第一次 upsert（插入）- 注意：Inserts 和 Updates 都会合并到 INSERT VALUES 中
	// Updates 中的值会覆盖 Inserts 中的同名字段
	result, err := lite.Upsert(ctx, testTable, UpsertRow{
		Id: "test_upsert_1",
		Inserts: []Update{
			{Field: "score", Value: 100}, // 只有 score 在 Inserts
		},
		Updates: []Update{
			{Field: "name", Value: "Initial2"}, // name 只在 Updates
		},
	})
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}
	if result.Id != "test_upsert_1" {
		t.Errorf("expected ID 'test_upsert_1', got %s", result.Id)
	}

	// 验证第一次插入
	ref, _ := lite.Get(ctx, testTable, "test_upsert_1")
	data := ref.Data.(map[string]interface{})
	if data["name"] != "Initial" {
		t.Errorf("expected name 'Initial', got %v", data["name"])
	}
	if int(data["score"].(int64)) != 100 {
		t.Errorf("expected score 100, got %v", data["score"])
	}

	// 第二次 upsert（更新）- 记录已存在，触发 ON DUPLICATE KEY UPDATE
	_, err = lite.Upsert(ctx, testTable, UpsertRow{
		Id: "test_upsert_1",
		Inserts: []Update{
			{Field: "score", Value: 200}, // 不会使用，因为记录已存在
		},
		Updates: []Update{
			{Field: "name", Value: "Updated2"},
			{Field: "score", Op: "incr", Value: 150},
		},
	})
	if err != nil {
		t.Fatalf("Upsert (update) failed: %v", err)
	}

	// 验证更新
	ref, _ = lite.Get(ctx, testTable, "test_upsert_1")
	data = ref.Data.(map[string]interface{})
	if data["name"] != "Updated" {
		t.Errorf("expected name 'Updated', got %v", data["name"])
	}
	// score 应该是 100 + 50 = 150
	if int(data["score"].(int64)) != 150 {
		t.Errorf("expected score 150, got %v", data["score"])
	}

	t.Log("TestMysqlLite_Upsert passed")
}

// TestMysqlLite_BatchUpsert 测试批量更新或插入（CASE WHEN）
func TestMysqlLite_BatchUpsert(t *testing.T) {
	lite := setupTestDB(t)
	defer cleanupTestDB(t, lite)

	ctx := context.Background()

	// 先插入一条数据
	_, _ = lite.Create(ctx, testTable, &TestRow{
		Id:    "batch_upsert_1",
		Name:  "Existing",
		Score: 50,
	})

	// 批量 upsert（CASE WHEN）
	// 注意：对于新插入的记录，Inserts 和 Updates（非 incr）都会合并到 INSERT VALUES
	// Updates 中的值会覆盖 Inserts 中的同名字段
	rows := []UpsertRow{
		{
			Id: "batch_upsert_1", // 已存在，应更新
			Inserts: []Update{
				{Field: "score", Value: 100}, // 不会使用
			},
			Updates: []Update{
				{Field: "name", Value: "Updated1"},
				{Field: "score", Op: "incr", Value: 10},
			},
		},
		{
			Id: "batch_upsert_2", // 不存在，应插入
			Inserts: []Update{
				{Field: "score", Value: 200},
			},
			Updates: []Update{
				{Field: "name", Value: "New2"}, // 会被合并到 INSERT VALUES
			},
		},
		{
			Id: "batch_upsert_3", // 不存在，应插入
			Inserts: []Update{
				{Field: "score", Value: 300},
			},
			Updates: []Update{
				{Field: "name", Value: "New3"}, // 会被合并到 INSERT VALUES
			},
		},
	}

	_, err := lite.BatchUpsert(ctx, testTable, rows)
	if err != nil {
		t.Fatalf("BatchUpsert failed: %v", err)
	}

	// 验证
	count, _ := lite.Count(ctx, testTable, nil)
	if count != 3 {
		t.Errorf("expected count 3, got %d", count)
	}

	// 验证更新的记录
	ref, _ := lite.Get(ctx, testTable, "batch_upsert_1")
	data := ref.Data.(map[string]interface{})
	if data["name"] != "Updated1" {
		t.Errorf("expected name 'Updated1', got %v", data["name"])
	}
	// score 应该是 50 + 10 = 60
	if int(data["score"].(int64)) != 60 {
		t.Errorf("expected score 60, got %v", data["score"])
	}

	// 验证新插入的记录
	ref, _ = lite.Get(ctx, testTable, "batch_upsert_2")
	data = ref.Data.(map[string]interface{})
	if data["name"] != "New2" {
		t.Errorf("expected name 'New2', got %v", data["name"])
	}
	if int(data["score"].(int64)) != 200 {
		t.Errorf("expected score 200, got %v", data["score"])
	}

	t.Log("TestMysqlLite_BatchUpsert passed")
}

// TestMysqlLite_UpsertSingleField 测试单字段更新或插入
func TestMysqlLite_UpsertSingleField(t *testing.T) {
	lite := setupTestDB(t)
	defer cleanupTestDB(t, lite)

	ctx := context.Background()

	// 测试插入
	err := lite.UpsertSingleField(ctx, testTable, UpsertSingleFields{
		Id: "test_single_field_1",
		Fields: map[string]interface{}{
			"name": "TestName",
			"age":  30,
		},
	})
	if err != nil {
		t.Fatalf("UpsertSingleField failed: %v", err)
	}

	// 验证
	ref, _ := lite.Get(ctx, testTable, "test_single_field_1")
	data := ref.Data.(map[string]interface{})
	if data["name"] != "TestName" {
		t.Errorf("expected name 'TestName', got %v", data["name"])
	}

	// 测试更新
	err = lite.UpsertSingleField(ctx, testTable, UpsertSingleFields{
		Id: "test_single_field_1",
		Fields: map[string]interface{}{
			"name": "UpdatedName",
		},
	})
	if err != nil {
		t.Fatalf("UpsertSingleField (update) failed: %v", err)
	}

	ref, _ = lite.Get(ctx, testTable, "test_single_field_1")
	data = ref.Data.(map[string]interface{})
	if data["name"] != "UpdatedName" {
		t.Errorf("expected name 'UpdatedName', got %v", data["name"])
	}

	t.Log("TestMysqlLite_UpsertSingleField passed")
}

// TestMysqlLite_ToUpdateOne 测试结构体转 UpdateOne
func TestMysqlLite_ToUpdateOne(t *testing.T) {
	lite := setupTestDB(t)
	defer cleanupTestDB(t, lite)

	row := &TestRow{
		Id:   "test_to_update_1",
		Name: "TestName",
		Age:  25,
	}

	upData := lite.ToUpdateOne(row, nil)
	if upData.ID != row.Id {
		t.Errorf("expected ID %s, got %s", row.Id, upData.ID)
	}
	if len(upData.Updates) == 0 {
		t.Error("expected non-empty updates")
	}

	// 检查是否包含 name 字段
	hasName := false
	for _, u := range upData.Updates {
		if u.Field == "name" && u.Value == "TestName" {
			hasName = true
			break
		}
	}
	if !hasName {
		t.Error("expected updates to contain name field")
	}

	t.Log("TestMysqlLite_ToUpdateOne passed")
}

// TestMysqlLite_QuoteField 测试字段名转义
func TestMysqlLite_QuoteField(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"name", "`name`"},
		{"u.name", "u.`name`"},
		{"`name`", "`name`"},
		{"u.`name`", "u.`name`"},
	}

	for _, tt := range tests {
		result := quoteField(tt.input)
		if result != tt.expected {
			t.Errorf("quoteField(%s) = %s, expected %s", tt.input, result, tt.expected)
		}
	}

	t.Log("TestMysqlLite_QuoteField passed")
}

// TestMysqlLite_BuildSQLCondsLite 测试条件构建
func TestMysqlLite_BuildSQLCondsLite(t *testing.T) {
	// 测试各种条件
	conds := Conds{
		{Field: "name", Cond: "=", Value: "test"},
		{Field: "age", Cond: ">", Value: 18},
		{Field: "status", Cond: "in", Value: []int{1, 2, 3}},
		{Field: "type", Cond: "like", Value: "%test%"},
	}

	where, args := buildSQLCondsLite(conds)
	if where == "" {
		t.Error("expected non-empty where clause")
	}
	if len(args) == 0 {
		t.Error("expected non-empty args")
	}

	t.Logf("WHERE: %s", where)
	t.Logf("Args: %v", args)
	t.Log("TestMysqlLite_BuildSQLCondsLite passed")
}
