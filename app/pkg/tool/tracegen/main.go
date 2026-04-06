package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

var (
	interfaceName = flag.String("interface", "", "Interface name to generate tracer for (e.g., DB)")
	packagePath   = flag.String("package", "", "Package path containing the interface (e.g., gitlab.../seago/utils/db)")
	outputFile    = flag.String("output", "", "Output file path (e.g., utils/db/db_traced.gen.go)")
	spanKind      = flag.String("kind", "client", "Span kind: client, server, producer, consumer, internal")
	componentType = flag.String("component", "generic", "Component type: db, cache, http, mq, generic")
	tracerPrefix  = flag.String("prefix", "", "Custom tracer name prefix (default: seago/utils/<package>)")
)

func main() {
	flag.Parse()

	if *interfaceName == "" || *packagePath == "" || *outputFile == "" {
		log.Fatal("Usage: tracegen -interface=<name> -package=<path> -output=<file> [-kind=<client|server|...>] [-component=<db|cache|...>]")
	}

	// 解析接口
	iface, err := parseInterface(*packagePath, *interfaceName)
	if err != nil {
		log.Fatalf("Failed to parse interface: %v", err)
	}

	// 设置组件类型和 span kind
	iface.ComponentType = *componentType
	iface.SpanKind = *spanKind
	if *tracerPrefix != "" {
		iface.TracerPrefix = *tracerPrefix
	} else {
		iface.TracerPrefix = fmt.Sprintf("seago/utils/%s", iface.PackageName)
	}

	// 生成代码
	if err := generateTracer(iface, *outputFile); err != nil {
		log.Fatalf("Failed to generate tracer: %v", err)
	}

	fmt.Printf("✅ Generated tracer for %s.%s -> %s\n", iface.PackageName, iface.InterfaceName, *outputFile)
	fmt.Printf("   Component: %s, SpanKind: %s\n", iface.ComponentType, iface.SpanKind)
}

// InterfaceDef 接口定义
type InterfaceDef struct {
	PackageName   string
	InterfaceName string
	Methods       []MethodDef
	ImportPath    string
	ComponentType string // db, cache, http, mq, generic
	SpanKind      string // client, server, producer, consumer, internal
	TracerPrefix  string // tracer name prefix
}

// MethodDef 方法定义
type MethodDef struct {
	Name       string
	Params     string       // 完整参数列表（带类型）
	ParamNames string       // 参数名称列表（不带类型）
	ParamList  []ParamInfo  // 参数详细信息列表
	Results    string       // 返回值列表
	ReturnVars string       // 返回变量名
	ErrorVar   string       // 错误变量名
	HasContext bool         // 第一个参数是否是 context.Context
	HasError   bool         // 最后一个返回值是否是 error
}

// ParamInfo 参数信息
type ParamInfo struct {
	Name string
	Type string
}

// parseInterface 解析接口定义
func parseInterface(pkgPath, ifaceName string) (*InterfaceDef, error) {
	// 获取包路径对应的文件系统路径
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		gopath = filepath.Join(os.Getenv("HOME"), "go")
	}

	// 尝试多个可能的路径
	var pkgDir string
	possiblePaths := []string{
		filepath.Join(gopath, "src", pkgPath),
		filepath.Join(".", strings.TrimPrefix(pkgPath, "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/")),
		pkgPath,
	}

	for _, p := range possiblePaths {
		if _, err := os.Stat(p); err == nil {
			pkgDir = p
			break
		}
	}

	if pkgDir == "" {
		return nil, fmt.Errorf("package directory not found for %s", pkgPath)
	}

	// 解析包中的所有 .go 文件
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, pkgDir, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse package: %w", err)
	}

	// 查找接口定义
	for pkgName, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				genDecl, ok := decl.(*ast.GenDecl)
				if !ok || genDecl.Tok != token.TYPE {
					continue
				}

				for _, spec := range genDecl.Specs {
					typeSpec, ok := spec.(*ast.TypeSpec)
					if !ok || typeSpec.Name.Name != ifaceName {
						continue
					}

					interfaceType, ok := typeSpec.Type.(*ast.InterfaceType)
					if !ok {
						continue
					}

					// 找到了接口，解析方法
					return parseInterfaceMethods(pkgName, ifaceName, interfaceType, pkgPath), nil
				}
			}
		}
	}

	return nil, fmt.Errorf("interface %s not found in package %s", ifaceName, pkgPath)
}

// parseInterfaceMethods 解析接口的所有方法
func parseInterfaceMethods(pkgName, ifaceName string, iface *ast.InterfaceType, importPath string) *InterfaceDef {
	def := &InterfaceDef{
		PackageName:   pkgName,
		InterfaceName: ifaceName,
		ImportPath:    importPath,
		Methods:       make([]MethodDef, 0),
	}

	for _, method := range iface.Methods.List {
		funcType, ok := method.Type.(*ast.FuncType)
		if !ok {
			continue
		}

		// 跳过嵌入的接口
		if len(method.Names) == 0 {
			continue
		}

		methodName := method.Names[0].Name
		methodDef := MethodDef{
			Name: methodName,
		}

		// 解析参数
		params := make([]string, 0)
		paramNames := make([]string, 0)
		paramList := make([]ParamInfo, 0)
		if funcType.Params != nil {
			for i, param := range funcType.Params.List {
				paramType := exprToString(param.Type)

				// 检查第一个参数是否是 context.Context
				if i == 0 && paramType == "context.Context" {
					methodDef.HasContext = true
				}

				if len(param.Names) > 0 {
					for _, name := range param.Names {
						paramName := name.Name
						params = append(params, fmt.Sprintf("%s %s", paramName, paramType))
						paramNames = append(paramNames, paramName)
						paramList = append(paramList, ParamInfo{
							Name: paramName,
							Type: paramType,
						})
					}
				} else {
					// 没有参数名，生成一个
					paramName := fmt.Sprintf("arg%d", i)
					params = append(params, fmt.Sprintf("%s %s", paramName, paramType))
					paramNames = append(paramNames, paramName)
					paramList = append(paramList, ParamInfo{
						Name: paramName,
						Type: paramType,
					})
				}
			}
		}
		methodDef.Params = strings.Join(params, ", ")
		methodDef.ParamNames = strings.Join(paramNames, ", ")
		methodDef.ParamList = paramList

		// 解析返回值
		results := make([]string, 0)
		returnVars := make([]string, 0)
		if funcType.Results != nil {
			for i, result := range funcType.Results.List {
				resultType := exprToString(result.Type)

				// 检查最后一个返回值是否是 error
				if i == len(funcType.Results.List)-1 && resultType == "error" {
					methodDef.HasError = true
				}

				if len(result.Names) > 0 {
					for _, name := range result.Names {
						results = append(results, resultType)
						returnVars = append(returnVars, name.Name)
					}
				} else {
					// 生成返回变量名
					returnVar := fmt.Sprintf("ret%d", i)
					results = append(results, resultType)
					returnVars = append(returnVars, returnVar)
				}
			}
		}

		if len(results) > 0 {
			methodDef.Results = "(" + strings.Join(results, ", ") + ")"
			methodDef.ReturnVars = strings.Join(returnVars, ", ")
			// 如果有错误，记录错误变量名
			if methodDef.HasError && len(returnVars) > 0 {
				methodDef.ErrorVar = returnVars[len(returnVars)-1]
			}
		}

		def.Methods = append(def.Methods, methodDef)
	}

	return def
}

// exprToString 将表达式转换为字符串
func exprToString(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return exprToString(e.X) + "." + e.Sel.Name
	case *ast.StarExpr:
		return "*" + exprToString(e.X)
	case *ast.ArrayType:
		return "[]" + exprToString(e.Elt)
	case *ast.MapType:
		return "map[" + exprToString(e.Key) + "]" + exprToString(e.Value)
	case *ast.Ellipsis:
		return "..." + exprToString(e.Elt)
	case *ast.InterfaceType:
		if e.Methods == nil || len(e.Methods.List) == 0 {
			return "interface{}"
		}
		return "interface{...}"
	default:
		return fmt.Sprintf("%T", expr)
	}
}

// generateTracer 生成追踪代理代码
func generateTracer(iface *InterfaceDef, outputPath string) error {
	tmpl := template.Must(template.New("tracer").Funcs(template.FuncMap{
		"spanKindConst": getSpanKindConst,
		"getAttributes": getAttributes,
	}).Parse(tracerTemplate))

	// 创建输出文件
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	outFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	// 执行模板
	return tmpl.Execute(outFile, iface)
}

// getSpanKindConst 获取 SpanKind 常量
func getSpanKindConst(spanKind string) string {
	switch strings.ToLower(spanKind) {
	case "server":
		return "trace.SpanKindServer"
	case "client":
		return "trace.SpanKindClient"
	case "producer":
		return "trace.SpanKindProducer"
	case "consumer":
		return "trace.SpanKindConsumer"
	case "internal":
		return "trace.SpanKindInternal"
	default:
		return "trace.SpanKindClient"
	}
}

// getAttributes 根据组件类型和参数生成属性
func getAttributes(componentType, methodName string, params []ParamInfo) string {
	attrs := make([]string, 0)

	switch strings.ToLower(componentType) {
	case "db":
		attrs = append(attrs, `attribute.String("db.system", "database")`)
		attrs = append(attrs, fmt.Sprintf(`attribute.String("db.operation", "%s")`, methodName))

		// 提取数据库相关参数
		for _, p := range params {
			pnameLower := strings.ToLower(p.Name)
			if pnameLower == "table" || pnameLower == "tablename" {
				attrs = append(attrs, fmt.Sprintf(`attribute.String("db.table", %s)`, p.Name))
			} else if pnameLower == "id" || pnameLower == "docid" || pnameLower == "documentid" {
				attrs = append(attrs, fmt.Sprintf(`attribute.String("db.id", %s)`, p.Name))
			} else if pnameLower == "conds" || pnameLower == "conditions" {
				attrs = append(attrs, fmt.Sprintf(`attribute.Int("db.conditions.count", len(%s))`, p.Name))
			} else if strings.Contains(pnameLower, "row") && strings.Contains(p.Type, "[]") {
				// 批量操作
				attrs = append(attrs, fmt.Sprintf(`attribute.Int("db.batch.size", len(%s))`, p.Name))
			} else if (pnameLower == "ids" || pnameLower == "updates") && strings.Contains(p.Type, "[]") {
				attrs = append(attrs, fmt.Sprintf(`attribute.Int("db.batch.size", len(%s))`, p.Name))
			}
		}

	case "cache":
		attrs = append(attrs, `attribute.String("cache.system", "cache")`)
		attrs = append(attrs, fmt.Sprintf(`attribute.String("cache.operation", "%s")`, methodName))

		// 提取缓存相关参数
		for _, p := range params {
			pnameLower := strings.ToLower(p.Name)
			if pnameLower == "key" || pnameLower == "k" {
				attrs = append(attrs, fmt.Sprintf(`attribute.String("cache.key", %s)`, p.Name))
			} else if pnameLower == "value" || pnameLower == "val" || pnameLower == "v" {
				// 值可能很大，记录类型和长度
				if p.Type == "string" {
					attrs = append(attrs, fmt.Sprintf(`attribute.Int("cache.value.length", len(%s))`, p.Name))
				}
			} else if pnameLower == "ttl" || pnameLower == "expiration" {
				attrs = append(attrs, fmt.Sprintf(`attribute.String("cache.ttl", %s.String())`, p.Name))
			} else if pnameLower == "cmd" || pnameLower == "command" {
				attrs = append(attrs, fmt.Sprintf(`attribute.String("cache.command", %s)`, p.Name))
			} else if pnameLower == "keys" && strings.Contains(p.Type, "[]") {
				attrs = append(attrs, fmt.Sprintf(`attribute.Int("cache.keys.count", len(%s))`, p.Name))
			}
		}

	case "http":
		attrs = append(attrs, fmt.Sprintf(`attribute.String("http.operation", "%s")`, methodName))

		// 提取 HTTP 相关参数
		for _, p := range params {
			pnameLower := strings.ToLower(p.Name)
			if pnameLower == "url" || pnameLower == "uri" {
				attrs = append(attrs, fmt.Sprintf(`attribute.String("http.url", %s)`, p.Name))
			} else if pnameLower == "method" {
				attrs = append(attrs, fmt.Sprintf(`attribute.String("http.method", %s)`, p.Name))
			} else if pnameLower == "path" {
				attrs = append(attrs, fmt.Sprintf(`attribute.String("http.target", %s)`, p.Name))
			} else if pnameLower == "status" || pnameLower == "statuscode" {
				attrs = append(attrs, fmt.Sprintf(`attribute.Int("http.status_code", %s)`, p.Name))
			} else if pnameLower == "headers" {
				attrs = append(attrs, fmt.Sprintf(`attribute.Int("http.headers.count", len(%s))`, p.Name))
			}
		}

	case "mq":
		attrs = append(attrs, `attribute.String("messaging.system", "mq")`)
		attrs = append(attrs, fmt.Sprintf(`attribute.String("messaging.operation", "%s")`, methodName))

		// 提取消息队列相关参数
		for _, p := range params {
			pnameLower := strings.ToLower(p.Name)
			if pnameLower == "topic" {
				attrs = append(attrs, fmt.Sprintf(`attribute.String("messaging.destination", %s)`, p.Name))
			} else if pnameLower == "messageid" || pnameLower == "msgid" {
				attrs = append(attrs, fmt.Sprintf(`attribute.String("messaging.message_id", %s)`, p.Name))
			} else if pnameLower == "message" || pnameLower == "msg" {
				attrs = append(attrs, `attribute.String("messaging.message.type", "message")`)
			} else if pnameLower == "key" {
				attrs = append(attrs, fmt.Sprintf(`attribute.String("messaging.kafka.message_key", %s)`, p.Name))
			}
		}

	default:
		attrs = append(attrs, fmt.Sprintf(`attribute.String("operation", "%s")`, methodName))

		// 通用参数提取
		for _, p := range params {
			pnameLower := strings.ToLower(p.Name)
			// 记录字符串类型的常见参数
			if p.Type == "string" && (pnameLower == "id" || pnameLower == "name" || pnameLower == "key") {
				attrs = append(attrs, fmt.Sprintf(`attribute.String("%s", %s)`, p.Name, p.Name))
			}
		}
	}

	return strings.Join(attrs, ",\n\t\t")
}

const tracerTemplate = `// Code generated by tracegen. DO NOT EDIT.

package {{.PackageName}}

import (
	"context"
	"time"

	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_ytrace"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	traced{{.InterfaceName}}TracerName = "{{.TracerPrefix}}"
)

// Traced{{.InterfaceName}} {{.InterfaceName}} 接口的追踪代理
// Component: {{.ComponentType}}, SpanKind: {{.SpanKind}}
type Traced{{.InterfaceName}} struct {
	underlying {{.InterfaceName}}
	tracer     trace.Tracer
}

// NewTraced{{.InterfaceName}} 创建追踪代理
func NewTraced{{.InterfaceName}}(underlying {{.InterfaceName}}) *Traced{{.InterfaceName}} {
	return &Traced{{.InterfaceName}}{
		underlying: underlying,
	}
}

// getTracer 获取 tracer（延迟获取，确保追踪系统已初始化）
func (t *Traced{{.InterfaceName}}) getTracer() trace.Tracer {
	return otel.Tracer(traced{{.InterfaceName}}TracerName)
}

{{range .Methods}}
// {{.Name}} 追踪代理方法
func (t *Traced{{$.InterfaceName}}) {{.Name}}({{.Params}}) {{.Results}} {
	{{if .HasContext}}
	// 检查追踪是否启用
	if !su_ytrace.IsEnabled() {
		// 追踪未启用，直接调用底层方法
		{{if .ReturnVars}}return {{end}}t.underlying.{{.Name}}({{.ParamNames}})
	}

	tracer := t.getTracer()

	// 提取 span（支持 Worker 和 Fiber 服务）
	ctx = su_ytrace.ExtractSpanFromContext(ctx)

	ctx, span := tracer.Start(ctx, "{{$.PackageName}}.{{.Name}}",
		trace.WithSpanKind({{spanKindConst $.SpanKind}}),
	)
	defer su_ytrace.SafeEndSpan(span)
	su_ytrace.MaybeSetRequestID(ctx, span)

	span.SetAttributes(
		{{getAttributes $.ComponentType .Name .ParamList}},
	)
	{{end}}

	{{if .ReturnVars}}{{.ReturnVars}} := {{end}}t.underlying.{{.Name}}({{.ParamNames}})

	{{if and .HasContext .HasError}}
	if {{.ErrorVar}} != nil {
		span.RecordError({{.ErrorVar}})
		span.SetStatus(codes.Error, {{.ErrorVar}}.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}
	{{end}}

	{{if .ReturnVars}}return {{.ReturnVars}}{{end}}
}
{{end}}
`
