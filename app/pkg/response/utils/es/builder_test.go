package es

import (
	"fmt"
	"strings"
	"testing"

	es "github.com/elastic/go-elasticsearch/v8"
)

func TestBuilderWithBoostAndMatchPhrase(t *testing.T) {
	// 创建一个模拟的 ES 客户端（这里只是为了测试，实际使用时需要真实的客户端）
	cfg := es.Config{
		Addresses: []string{"http://localhost:9200"},
	}
	client, err := es.NewClient(cfg)
	if err != nil {
		t.Skipf("无法创建 ES 客户端，跳过测试: %v", err)
	}

	builder := NewBuilder(client)

	// 测试 match_phrase 查询
	builder.Must("title", "match_phrase", "elasticsearch", 2.0)

	// 测试带 boost 的 match 查询
	builder.Must("content", "match", "search engine", 1.5)

	// 测试带 boost 的 term 查询
	builder.Filter("status", "=", "active", 1.2)

	// 测试带 boost 的 range 查询
	builder.Must("age", ">=", 18, 0.8)

	// 测试 must_not 查询
	builder.MustNot("category", "=", "deprecated", 0.5)

	// 构建查询
	query := builder.BuildQuery()

	// 打印查询 JSON
	fmt.Println("=== ES Query JSON ===")
	fmt.Println(query)
	fmt.Println("====================")

	// 验证查询包含预期的内容
	if query == "" {
		t.Error("查询字符串不能为空")
	}

	// 验证是否包含 match_phrase
	if !strings.Contains(query, "match_phrase") {
		t.Error("查询应该包含 match_phrase")
	}

	// 验证是否包含 boost 参数
	if !strings.Contains(query, "boost") {
		t.Error("查询应该包含 boost 参数")
	}

	// 验证是否包含 title 字段
	if !strings.Contains(query, "title") {
		t.Error("查询应该包含 title 字段")
	}

	// 验证是否包含 content 字段
	if !strings.Contains(query, "content") {
		t.Error("查询应该包含 content 字段")
	}
}

func TestBuilderWhereMethod(t *testing.T) {
	cfg := es.Config{
		Addresses: []string{"http://localhost:9200"},
	}
	client, err := es.NewClient(cfg)
	if err != nil {
		t.Skipf("无法创建 ES 客户端，跳过测试: %v", err)
	}

	builder := NewBuilder(client)

	// 测试 Where 方法的各种条件
	builder.Where("name", "match_phrase", "john doe", 3.0)
	builder.Where("age", ">=", 25, 1.5)
	builder.Where("status", "=", "active", 1.0)

	query := builder.BuildQuery()
	fmt.Println("=== Where Method Test ===")
	fmt.Println(query)
	fmt.Println("=========================")

	if !strings.Contains(query, "match_phrase") {
		t.Error("Where 方法应该支持 match_phrase")
	}

	if !strings.Contains(query, "boost") {
		t.Error("Where 方法应该支持 boost 参数")
	}
}

func TestBuilderComplexQuery(t *testing.T) {
	cfg := es.Config{
		Addresses: []string{"http://localhost:9200"},
	}
	client, err := es.NewClient(cfg)
	if err != nil {
		t.Skipf("无法创建 ES 客户端，跳过测试: %v", err)
	}

	builder := NewBuilder(client)

	// 构建一个复杂的查询
	builder.Index("test_index")
	builder.Must("title", "match_phrase", "elasticsearch tutorial", 2.5)
	builder.Must("content", "match", "search engine", 1.8)
	builder.Filter("status", "=", "published", 1.2)
	builder.Must("publish_date", ">=", "2023-01-01", 1.0)
	builder.MustNot("category", "=", "archived", 0.3)
	builder.Size(10)
	builder.SetFrom(0)

	query := builder.BuildQuery()
	fmt.Println("=== Complex Query Test ===")
	fmt.Println(query)
	fmt.Println("=========================")

	// 验证查询结构
	if !strings.Contains(query, `"query":{"bool"`) {
		t.Error("查询应该包含 bool 查询结构")
	}

	if !strings.Contains(query, `"must"`) {
		t.Error("查询应该包含 must 条件")
	}

	if !strings.Contains(query, `"filter"`) {
		t.Error("查询应该包含 filter 条件")
	}

	if !strings.Contains(query, `"must_not"`) {
		t.Error("查询应该包含 must_not 条件")
	}
}
