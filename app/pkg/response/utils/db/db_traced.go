package db

import (
	"context"
	"fmt"
	"strings"

	jsoniter "github.com/json-iterator/go"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_ytrace"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	tracedDBTracerName = "seago/utils/db"
	// 最大语句长度限制，避免 span attribute 过大
	maxStatementLen = 500
	// 最大值长度限制
	maxValueLen = 200
)

// formatValue 格式化单个值，限制长度
func formatValue(v interface{}) string {
	if v == nil {
		return "null"
	}
	s := fmt.Sprintf("%v", v)
	if len(s) > maxValueLen {
		return s[:maxValueLen] + "..."
	}
	return s
}

// formatConds 格式化查询条件为可读字符串
// 示例输出: "status == active AND user_id == 12345 AND created_at > 2024-01-01"
func formatConds(conds Conds) string {
	if len(conds) == 0 {
		return ""
	}

	var parts []string
	for _, cond := range conds {
		op := cond.Cond
		if op == "" {
			op = "=="
		}
		parts = append(parts, fmt.Sprintf("%s %s %s", cond.Field, op, formatValue(cond.Value)))
	}

	result := strings.Join(parts, " AND ")
	if len(result) > maxStatementLen {
		return result[:maxStatementLen] + "..."
	}
	return result
}

// formatUpdates 格式化更新操作为可读字符串
// 示例输出: "status = deleted, view_count incr 1, updated_at = 2024-01-01"
func formatUpdates(updates []Update) string {
	if len(updates) == 0 {
		return ""
	}

	var parts []string
	for _, u := range updates {
		op := u.Op
		if op == "" {
			op = "="
		}
		parts = append(parts, fmt.Sprintf("%s %s %s", u.Field, op, formatValue(u.Value)))
	}

	result := strings.Join(parts, ", ")
	if len(result) > maxStatementLen {
		return result[:maxStatementLen] + "..."
	}
	return result
}

// formatRow 格式化 Row 数据为 JSON 字符串（用于 Insert/Create）
func formatRow(row Row) string {
	if row == nil {
		return ""
	}

	// 尝试序列化为 JSON
	data, err := jsoniter.Marshal(row)
	if err != nil {
		return fmt.Sprintf("{id: %s, error: %v}", row.ID(), err)
	}

	result := string(data)
	if len(result) > maxStatementLen {
		return result[:maxStatementLen] + "..."
	}
	return result
}

// formatUpsertRow 格式化 UpsertRow 为可读字符串
func formatUpsertRow(row UpsertRow) string {
	var parts []string

	if len(row.Updates) > 0 {
		parts = append(parts, fmt.Sprintf("ON_UPDATE: %s", formatUpdates(row.Updates)))
	}
	if len(row.Inserts) > 0 {
		parts = append(parts, fmt.Sprintf("ON_INSERT: %s", formatUpdates(row.Inserts)))
	}

	result := strings.Join(parts, "; ")
	if len(result) > maxStatementLen {
		return result[:maxStatementLen] + "..."
	}
	return result
}

// formatSingleFields 格式化 UpsertSingleFields 的 Fields 为可读字符串
// 示例输出: "name = John, age = 25, status = active"
func formatSingleFields(fields map[string]any) string {
	if len(fields) == 0 {
		return ""
	}

	var parts []string
	for fieldName, fieldValue := range fields {
		parts = append(parts, fmt.Sprintf("%s = %s", fieldName, formatValue(fieldValue)))
	}

	result := strings.Join(parts, ", ")
	if len(result) > maxStatementLen {
		return result[:maxStatementLen] + "..."
	}
	return result
}

// TracedDB DB 接口的追踪代理
// 使用结构体嵌入模式，零业务代码改动
type TracedDB struct {
	underlying DB
	tracer     trace.Tracer
	dbSystem   string // mongodb, firestore, mysql, elasticsearch
}

// NewTracedDB 创建追踪代理
func NewTracedDB(underlying DB, dbSystem string) *TracedDB {
	return &TracedDB{
		underlying: underlying,
		dbSystem:   dbSystem,
	}
}

// getTracer 获取 tracer（延迟获取，确保追踪系统已初始化）
func (t *TracedDB) getTracer() trace.Tracer {
	return otel.Tracer(tracedDBTracerName)
}

// Insert 追踪代理方法
func (t *TracedDB) Insert(ctx context.Context, table string, row Row, options ...*InsertOptions) (string, error) {
	// 检查追踪是否启用
	if !su_ytrace.IsEnabled() {
		// 追踪未启用，直接调用底层方法
		return t.underlying.Insert(ctx, table, row, options...)
	}

	tracer := t.getTracer()

	// 提取 span（支持 Worker 和 Fiber 服务）

	ctx = su_ytrace.ExtractSpanFromContext(ctx)
	ctx, span := tracer.Start(ctx, "db.Insert",
		trace.WithSpanKind(trace.SpanKindClient),
	)
	defer su_ytrace.SafeEndSpan(span)
	su_ytrace.MaybeSetRequestID(ctx, span)

	attrs := []attribute.KeyValue{
		attribute.String("db.system", t.dbSystem),
		attribute.String("db.operation", "Insert"),
		attribute.String("db.table", table),
	}

	// 记录输入 row 的 ID 和完整数据
	if row != nil {
		if id := row.ID(); id != "" {
			attrs = append(attrs, attribute.String("db.input.id", id))
		}
		// 记录插入的完整数据（JSON 格式）
		if statement := formatRow(row); statement != "" {
			attrs = append(attrs, attribute.String("db.statement", statement))
		}
	}

	span.SetAttributes(attrs...)

	id, err := t.underlying.Insert(ctx, table, row, options...)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetAttributes(attribute.String("db.result.id", id))
		span.SetStatus(codes.Ok, "")
	}

	return id, err
}

// Delete 追踪代理方法
func (t *TracedDB) Delete(ctx context.Context, table string, id string, options ...*DeleteOptions) (int, error) {
	// 检查追踪是否启用
	if !su_ytrace.IsEnabled() {
		// 追踪未启用，直接调用底层方法
		return t.underlying.Delete(ctx, table, id, options...)
	}

	tracer := t.getTracer()

	// 提取 span（支持 Worker 和 Fiber 服务）

	ctx = su_ytrace.ExtractSpanFromContext(ctx)
	ctx, span := tracer.Start(ctx, "db.Delete",
		trace.WithSpanKind(trace.SpanKindClient),
	)
	defer su_ytrace.SafeEndSpan(span)
	su_ytrace.MaybeSetRequestID(ctx, span)

	span.SetAttributes(
		attribute.String("db.system", t.dbSystem),
		attribute.String("db.operation", "Delete"),
		attribute.String("db.table", table),
		attribute.String("db.id", id),
	)

	count, err := t.underlying.Delete(ctx, table, id, options...)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetAttributes(attribute.Int("db.result.count", count))
		span.SetStatus(codes.Ok, "")
	}

	return count, err
}

// Create 追踪代理方法
func (t *TracedDB) Create(ctx context.Context, table string, row Row, options ...*CreateOptions) (*CreateResult, error) {
	// 检查追踪是否启用
	if !su_ytrace.IsEnabled() {
		// 追踪未启用，直接调用底层方法
		return t.underlying.Create(ctx, table, row, options...)
	}

	tracer := t.getTracer()

	// 提取 span（支持 Worker 和 Fiber 服务）

	ctx = su_ytrace.ExtractSpanFromContext(ctx)
	ctx, span := tracer.Start(ctx, "db.Create",
		trace.WithSpanKind(trace.SpanKindClient),
	)
	defer su_ytrace.SafeEndSpan(span)
	su_ytrace.MaybeSetRequestID(ctx, span)

	attrs := []attribute.KeyValue{
		attribute.String("db.system", t.dbSystem),
		attribute.String("db.operation", "Create"),
		attribute.String("db.table", table),
	}

	// 记录输入 row 的 ID 和完整数据
	if row != nil {
		if id := row.ID(); id != "" {
			attrs = append(attrs, attribute.String("db.input.id", id))
		}
		// 记录插入的完整数据（JSON 格式）
		if statement := formatRow(row); statement != "" {
			attrs = append(attrs, attribute.String("db.statement", statement))
		}
	}

	span.SetAttributes(attrs...)

	result, err := t.underlying.Create(ctx, table, row, options...)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		if result != nil {
			span.SetAttributes(attribute.String("db.result.id", result.ID))
		}
		span.SetStatus(codes.Ok, "")
	}

	return result, err
}

// Update 追踪代理方法
func (t *TracedDB) Update(ctx context.Context, table string, update UpdateOne, options ...*UpdateOptions) error {
	// 检查追踪是否启用
	if !su_ytrace.IsEnabled() {
		// 追踪未启用，直接调用底层方法
		return t.underlying.Update(ctx, table, update, options...)
	}

	tracer := t.getTracer()

	// 提取 span（支持 Worker 和 Fiber 服务）

	ctx = su_ytrace.ExtractSpanFromContext(ctx)
	ctx, span := tracer.Start(ctx, "db.Update",
		trace.WithSpanKind(trace.SpanKindClient),
	)
	defer su_ytrace.SafeEndSpan(span)
	su_ytrace.MaybeSetRequestID(ctx, span)

	attrs := []attribute.KeyValue{
		attribute.String("db.system", t.dbSystem),
		attribute.String("db.operation", "Update"),
		attribute.String("db.table", table),
		attribute.String("db.id", update.ID),
	}

	// 记录更新字段数量和完整更新语句
	if update.Updates != nil {
		attrs = append(attrs, attribute.Int("db.update.fields", len(update.Updates)))
		// 记录完整的更新语句
		if statement := formatUpdates(update.Updates); statement != "" {
			attrs = append(attrs, attribute.String("db.statement", statement))
		}
	}

	span.SetAttributes(attrs...)

	err := t.underlying.Update(ctx, table, update, options...)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}

	return err
}

// Get 追踪代理方法
func (t *TracedDB) Get(ctx context.Context, table string, id string, options ...*GetOptions) (*DocumentRef, error) {
	// 检查追踪是否启用
	if !su_ytrace.IsEnabled() {
		// 追踪未启用，直接调用底层方法
		return t.underlying.Get(ctx, table, id, options...)
	}

	tracer := t.getTracer()

	// 提取 span（支持 Worker 和 Fiber 服务）

	ctx = su_ytrace.ExtractSpanFromContext(ctx)
	ctx, span := tracer.Start(ctx, "db.Get",
		trace.WithSpanKind(trace.SpanKindClient),
	)
	defer su_ytrace.SafeEndSpan(span)
	su_ytrace.MaybeSetRequestID(ctx, span)

	span.SetAttributes(
		attribute.String("db.system", t.dbSystem),
		attribute.String("db.operation", "Get"),
		attribute.String("db.table", table),
		attribute.String("db.id", id),
	)

	doc, err := t.underlying.Get(ctx, table, id, options...)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		// 记录是否找到文档
		span.SetAttributes(attribute.Bool("db.result.found", doc != nil))
		span.SetStatus(codes.Ok, "")
	}

	return doc, err
}

// Find 追踪代理方法
func (t *TracedDB) Find(ctx context.Context, table string, conds Conds, options ...*FindOptions) (*Iterator, error) {
	// 检查追踪是否启用
	if !su_ytrace.IsEnabled() {
		// 追踪未启用，直接调用底层方法
		return t.underlying.Find(ctx, table, conds, options...)
	}

	tracer := t.getTracer()

	// 提取 span（支持 Worker 和 Fiber 服务）

	ctx = su_ytrace.ExtractSpanFromContext(ctx)
	ctx, span := tracer.Start(ctx, "db.Find",
		trace.WithSpanKind(trace.SpanKindClient),
	)
	defer su_ytrace.SafeEndSpan(span)
	su_ytrace.MaybeSetRequestID(ctx, span)

	attrs := []attribute.KeyValue{
		attribute.String("db.system", t.dbSystem),
		attribute.String("db.operation", "Find"),
		attribute.String("db.table", table),
		attribute.Int("db.query.conditions", len(conds)),
	}

	// 记录完整的查询条件语句
	if len(conds) > 0 {
		// 格式: "status == active AND user_id == 12345 AND created_at > 2024-01-01"
		if statement := formatConds(conds); statement != "" {
			attrs = append(attrs, attribute.String("db.statement", statement))
		}
	}

	// 记录 options（如 limit、offset）
	if len(options) > 0 && options[0] != nil {
		opt := options[0]
		if opt.Limit > 0 {
			attrs = append(attrs, attribute.Int64("db.query.limit", opt.Limit))
		}
		if opt.Offset > 0 {
			attrs = append(attrs, attribute.Int64("db.query.offset", opt.Offset))
		}
	}

	span.SetAttributes(attrs...)

	iter, err := t.underlying.Find(ctx, table, conds, options...)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}

	return iter, err
}

// BatchCreate 追踪代理方法
func (t *TracedDB) BatchCreate(ctx context.Context, table string, rows []Row, options ...*BatchWriteOptions) *BatchWriteResult {
	// 检查追踪是否启用
	if !su_ytrace.IsEnabled() {
		// 追踪未启用，直接调用底层方法
		return t.underlying.BatchCreate(ctx, table, rows, options...)
	}

	tracer := t.getTracer()

	// 提取 span（支持 Worker 和 Fiber 服务）

	ctx = su_ytrace.ExtractSpanFromContext(ctx)
	ctx, span := tracer.Start(ctx, "db.BatchCreate",
		trace.WithSpanKind(trace.SpanKindClient),
	)
	defer su_ytrace.SafeEndSpan(span)
	su_ytrace.MaybeSetRequestID(ctx, span)

	attrs := []attribute.KeyValue{
		attribute.String("db.system", t.dbSystem),
		attribute.String("db.operation", "BatchCreate"),
		attribute.String("db.table", table),
		attribute.Int("db.batch.size", len(rows)),
	}

	// 记录第一条数据作为示例
	if len(rows) > 0 && rows[0] != nil {
		if statement := formatRow(rows[0]); statement != "" {
			attrs = append(attrs, attribute.String("db.statement.sample", statement))
		}
	}

	span.SetAttributes(attrs...)

	result := t.underlying.BatchCreate(ctx, table, rows, options...)

	if result != nil {
		if err := result.Error(); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		} else {
			span.SetAttributes(attribute.Int64("db.result.affected", result.Affected))
			span.SetStatus(codes.Ok, "")
		}
	} else {
		span.SetStatus(codes.Ok, "")
	}

	return result
}

// BatchInsert 追踪代理方法
func (t *TracedDB) BatchInsert(ctx context.Context, table string, rows []Row, options ...*BatchWriteOptions) *BatchWriteResult {
	// 检查追踪是否启用
	if !su_ytrace.IsEnabled() {
		// 追踪未启用，直接调用底层方法
		return t.underlying.BatchInsert(ctx, table, rows, options...)
	}

	tracer := t.getTracer()

	// 提取 span（支持 Worker 和 Fiber 服务）

	ctx = su_ytrace.ExtractSpanFromContext(ctx)
	ctx, span := tracer.Start(ctx, "db.BatchInsert",
		trace.WithSpanKind(trace.SpanKindClient),
	)
	defer su_ytrace.SafeEndSpan(span)
	su_ytrace.MaybeSetRequestID(ctx, span)

	attrs := []attribute.KeyValue{
		attribute.String("db.system", t.dbSystem),
		attribute.String("db.operation", "BatchInsert"),
		attribute.String("db.table", table),
		attribute.Int("db.batch.size", len(rows)),
	}

	// 记录第一条数据作为示例
	if len(rows) > 0 && rows[0] != nil {
		if statement := formatRow(rows[0]); statement != "" {
			attrs = append(attrs, attribute.String("db.statement.sample", statement))
		}
	}

	span.SetAttributes(attrs...)

	result := t.underlying.BatchInsert(ctx, table, rows, options...)

	if result != nil {
		if err := result.Error(); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		} else {
			span.SetAttributes(attribute.Int64("db.result.affected", result.Affected))
			span.SetStatus(codes.Ok, "")
		}
	} else {
		span.SetStatus(codes.Ok, "")
	}

	return result
}

// BatchDelete 追踪代理方法
func (t *TracedDB) BatchDelete(ctx context.Context, table string, ids []string, options ...*BatchWriteOptions) *BatchWriteResult {
	// 检查追踪是否启用
	if !su_ytrace.IsEnabled() {
		// 追踪未启用，直接调用底层方法
		return t.underlying.BatchDelete(ctx, table, ids, options...)
	}

	tracer := t.getTracer()

	// 提取 span（支持 Worker 和 Fiber 服务）

	ctx = su_ytrace.ExtractSpanFromContext(ctx)
	ctx, span := tracer.Start(ctx, "db.BatchDelete",
		trace.WithSpanKind(trace.SpanKindClient),
	)
	defer su_ytrace.SafeEndSpan(span)
	su_ytrace.MaybeSetRequestID(ctx, span)

	attrs := []attribute.KeyValue{
		attribute.String("db.system", t.dbSystem),
		attribute.String("db.operation", "BatchDelete"),
		attribute.String("db.table", table),
		attribute.Int("db.batch.size", len(ids)),
	}

	// 记录 IDs（限制数量避免日志过大）
	if len(ids) > 0 {
		if len(ids) <= 10 {
			attrs = append(attrs, attribute.String("db.batch.ids", strings.Join(ids, ",")))
		} else {
			attrs = append(attrs, attribute.String("db.batch.ids", fmt.Sprintf("%s,... (%d more)", strings.Join(ids[:10], ","), len(ids)-10)))
		}
	}

	span.SetAttributes(attrs...)

	result := t.underlying.BatchDelete(ctx, table, ids, options...)

	if result != nil {
		if err := result.Error(); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		} else {
			span.SetAttributes(attribute.Int64("db.result.affected", result.Affected))
			span.SetStatus(codes.Ok, "")
		}
	} else {
		span.SetStatus(codes.Ok, "")
	}

	return result
}

// BatchUpdate 追踪代理方法
func (t *TracedDB) BatchUpdate(ctx context.Context, table string, updates []UpdateOne, options ...*BatchWriteOptions) *BatchWriteResult {
	// 检查追踪是否启用
	if !su_ytrace.IsEnabled() {
		// 追踪未启用，直接调用底层方法
		return t.underlying.BatchUpdate(ctx, table, updates, options...)
	}

	tracer := t.getTracer()

	// 提取 span（支持 Worker 和 Fiber 服务）

	ctx = su_ytrace.ExtractSpanFromContext(ctx)
	ctx, span := tracer.Start(ctx, "db.BatchUpdate",
		trace.WithSpanKind(trace.SpanKindClient),
	)
	defer su_ytrace.SafeEndSpan(span)
	su_ytrace.MaybeSetRequestID(ctx, span)

	attrs := []attribute.KeyValue{
		attribute.String("db.system", t.dbSystem),
		attribute.String("db.operation", "BatchUpdate"),
		attribute.String("db.table", table),
		attribute.Int("db.batch.size", len(updates)),
	}

	// 记录更新的 IDs 和第一条更新的语句示例
	if len(updates) > 0 {
		ids := make([]string, len(updates))
		for i, update := range updates {
			ids[i] = update.ID
		}
		if len(ids) <= 10 {
			attrs = append(attrs, attribute.String("db.batch.ids", strings.Join(ids, ",")))
		} else {
			attrs = append(attrs, attribute.String("db.batch.ids", fmt.Sprintf("%s,... (%d more)", strings.Join(ids[:10], ","), len(ids)-10)))
		}
		// 记录第一条更新的语句作为示例
		if len(updates[0].Updates) > 0 {
			if statement := formatUpdates(updates[0].Updates); statement != "" {
				attrs = append(attrs, attribute.String("db.statement.sample", statement))
			}
		}
	}

	span.SetAttributes(attrs...)

	result := t.underlying.BatchUpdate(ctx, table, updates, options...)

	if result != nil {
		if err := result.Error(); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		} else {
			span.SetAttributes(attribute.Int64("db.result.affected", result.Affected))
			span.SetStatus(codes.Ok, "")
		}
	} else {
		span.SetStatus(codes.Ok, "")
	}

	return result
}

// BatchGet 追踪代理方法
func (t *TracedDB) BatchGet(ctx context.Context, table string, ids []string, options ...*GetOptions) (*Iterator, error) {
	// 检查追踪是否启用
	if !su_ytrace.IsEnabled() {
		// 追踪未启用，直接调用底层方法
		return t.underlying.BatchGet(ctx, table, ids, options...)
	}

	tracer := t.getTracer()

	// 提取 span（支持 Worker 和 Fiber 服务）

	ctx = su_ytrace.ExtractSpanFromContext(ctx)
	ctx, span := tracer.Start(ctx, "db.BatchGet",
		trace.WithSpanKind(trace.SpanKindClient),
	)
	defer su_ytrace.SafeEndSpan(span)
	su_ytrace.MaybeSetRequestID(ctx, span)

	attrs := []attribute.KeyValue{
		attribute.String("db.system", t.dbSystem),
		attribute.String("db.operation", "BatchGet"),
		attribute.String("db.table", table),
		attribute.Int("db.batch.size", len(ids)),
	}

	// 记录 IDs（限制数量避免日志过大）
	if len(ids) > 0 {
		if len(ids) <= 10 {
			attrs = append(attrs, attribute.String("db.batch.ids", strings.Join(ids, ",")))
		} else {
			attrs = append(attrs, attribute.String("db.batch.ids", fmt.Sprintf("%s,... (%d more)", strings.Join(ids[:10], ","), len(ids)-10)))
		}
	}

	span.SetAttributes(attrs...)

	iter, err := t.underlying.BatchGet(ctx, table, ids, options...)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}

	return iter, err
}

// Count 追踪代理方法
func (t *TracedDB) Count(ctx context.Context, table string, conds Conds) (int64, error) {
	// 检查追踪是否启用
	if !su_ytrace.IsEnabled() {
		// 追踪未启用，直接调用底层方法
		return t.underlying.Count(ctx, table, conds)
	}

	tracer := t.getTracer()

	// 提取 span（支持 Worker 和 Fiber 服务）

	ctx = su_ytrace.ExtractSpanFromContext(ctx)
	ctx, span := tracer.Start(ctx, "db.Count",
		trace.WithSpanKind(trace.SpanKindClient),
	)
	defer su_ytrace.SafeEndSpan(span)
	su_ytrace.MaybeSetRequestID(ctx, span)

	attrs := []attribute.KeyValue{
		attribute.String("db.system", t.dbSystem),
		attribute.String("db.operation", "Count"),
		attribute.String("db.table", table),
		attribute.Int("db.query.conditions", len(conds)),
	}

	// 记录完整的查询条件语句
	if len(conds) > 0 {
		if statement := formatConds(conds); statement != "" {
			attrs = append(attrs, attribute.String("db.statement", statement))
		}
	}

	span.SetAttributes(attrs...)

	count, err := t.underlying.Count(ctx, table, conds)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetAttributes(attribute.Int64("db.result.count", count))
		span.SetStatus(codes.Ok, "")
	}

	return count, err
}

// Upsert 追踪代理方法
func (t *TracedDB) Upsert(ctx context.Context, table string, row UpsertRow, options ...*UpsertOptions) (*UpsertRs, error) {
	// 检查追踪是否启用
	if !su_ytrace.IsEnabled() {
		// 追踪未启用，直接调用底层方法
		return t.underlying.Upsert(ctx, table, row, options...)
	}

	tracer := t.getTracer()

	// 提取 span（支持 Worker 和 Fiber 服务）

	ctx = su_ytrace.ExtractSpanFromContext(ctx)
	ctx, span := tracer.Start(ctx, "db.Upsert",
		trace.WithSpanKind(trace.SpanKindClient),
	)
	defer su_ytrace.SafeEndSpan(span)
	su_ytrace.MaybeSetRequestID(ctx, span)

	attrs := []attribute.KeyValue{
		attribute.String("db.system", t.dbSystem),
		attribute.String("db.operation", "Upsert"),
		attribute.String("db.table", table),
		attribute.String("db.id", row.Id),
		attribute.Int("db.upsert.updates", len(row.Updates)),
		attribute.Int("db.upsert.inserts", len(row.Inserts)),
	}

	// 记录完整的 upsert 语句
	if statement := formatUpsertRow(row); statement != "" {
		attrs = append(attrs, attribute.String("db.statement", statement))
	}

	span.SetAttributes(attrs...)

	result, err := t.underlying.Upsert(ctx, table, row, options...)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		if result != nil {
			span.SetAttributes(
				attribute.String("db.result.id", result.Id),
				attribute.Int64("db.result.match_count", result.MatchCount),
			)
		}
		span.SetStatus(codes.Ok, "")
	}

	return result, err
}

// UpsertSingleField 追踪代理方法
func (t *TracedDB) UpsertSingleField(ctx context.Context, table string, row UpsertSingleFields, options ...*UpsertSingleFieldRowOptions) error {
	// 检查追踪是否启用
	if !su_ytrace.IsEnabled() {
		// 追踪未启用，直接调用底层方法
		return t.underlying.UpsertSingleField(ctx, table, row, options...)
	}

	tracer := t.getTracer()

	// 提取 span（支持 Worker 和 Fiber 服务）

	ctx = su_ytrace.ExtractSpanFromContext(ctx)
	ctx, span := tracer.Start(ctx, "db.UpsertSingleField",
		trace.WithSpanKind(trace.SpanKindClient),
	)
	defer su_ytrace.SafeEndSpan(span)
	su_ytrace.MaybeSetRequestID(ctx, span)

	attrs := []attribute.KeyValue{
		attribute.String("db.system", t.dbSystem),
		attribute.String("db.operation", "UpsertSingleField"),
		attribute.String("db.table", table),
		attribute.String("db.id", row.Id),
		attribute.Int("db.fields.count", len(row.Fields)),
	}

	// 记录字段名称和值
	if len(row.Fields) > 0 {
		// 记录完整的字段和值（格式: "name = John, age = 25"）
		if statement := formatSingleFields(row.Fields); statement != "" {
			attrs = append(attrs, attribute.String("db.statement", statement))
		}
	}

	span.SetAttributes(attrs...)

	err := t.underlying.UpsertSingleField(ctx, table, row, options...)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}

	return err
}

// BatchUpsert 追踪代理方法
func (t *TracedDB) BatchUpsert(ctx context.Context, table string, rows []UpsertRow, options ...*BatchWriteOptions) (*BatchUpsertRs, error) {
	// 检查追踪是否启用
	if !su_ytrace.IsEnabled() {
		// 追踪未启用，直接调用底层方法
		return t.underlying.BatchUpsert(ctx, table, rows, options...)
	}

	tracer := t.getTracer()

	// 提取 span（支持 Worker 和 Fiber 服务）

	ctx = su_ytrace.ExtractSpanFromContext(ctx)
	ctx, span := tracer.Start(ctx, "db.BatchUpsert",
		trace.WithSpanKind(trace.SpanKindClient),
	)
	defer su_ytrace.SafeEndSpan(span)
	su_ytrace.MaybeSetRequestID(ctx, span)

	attrs := []attribute.KeyValue{
		attribute.String("db.system", t.dbSystem),
		attribute.String("db.operation", "BatchUpsert"),
		attribute.String("db.table", table),
		attribute.Int("db.batch.size", len(rows)),
	}

	// 记录 IDs（限制数量避免日志过大）
	if len(rows) > 0 {
		ids := make([]string, len(rows))
		for i, row := range rows {
			ids[i] = row.Id
		}
		if len(ids) <= 10 {
			attrs = append(attrs, attribute.String("db.batch.ids", strings.Join(ids, ",")))
		} else {
			attrs = append(attrs, attribute.String("db.batch.ids", fmt.Sprintf("%s,... (%d more)", strings.Join(ids[:10], ","), len(ids)-10)))
		}
		// 记录第一条 upsert 语句作为示例
		if statement := formatUpsertRow(rows[0]); statement != "" {
			attrs = append(attrs, attribute.String("db.statement.sample", statement))
		}
	}

	span.SetAttributes(attrs...)

	result, err := t.underlying.BatchUpsert(ctx, table, rows, options...)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		if result != nil {
			span.SetAttributes(
				attribute.Int64("db.result.insert_count", result.InsertCount),
				attribute.Int64("db.result.update_count", result.UpdateCount),
			)
		}
		span.SetStatus(codes.Ok, "")
	}

	return result, err
}

// ToUpdateOne 追踪代理方法（实现 UpDateOne 接口）
func (t *TracedDB) ToUpdateOne(data any, options *UpdateOptions) UpdateOne {
	// 这个方法不需要追踪，直接调用底层实现
	return t.underlying.ToUpdateOne(data, options)
}
