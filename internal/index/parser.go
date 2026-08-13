package index

import (
	"crypto/md5"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"strings"

	"gogitor/internal/textutil"
)

// SymbolKind — тип символа верхнего уровня.
type SymbolKind int

const (
	KindFunc SymbolKind = iota
	KindMethod
	KindType
	KindStruct
	KindInterface
	KindConst
	KindVar
)

func (k SymbolKind) String() string {
	switch k {
	case KindFunc:
		return "func"
	case KindMethod:
		return "method"
	case KindType:
		return "type"
	case KindStruct:
		return "struct"
	case KindInterface:
		return "interface"
	case KindConst:
		return "const"
	case KindVar:
		return "var"
	default:
		return "unknown"
	}
}

// Symbol — объявленный символ верхнего уровня.
type Symbol struct {
	Name     string     `json:"name"`
	Kind     SymbolKind `json:"kind"`
	Receiver string     `json:"receiver,omitempty"`
	Line     int        `json:"line"`
	Exported bool       `json:"exported"`
	Doc      string     `json:"doc,omitempty"`
}

// CallEdge — вызов внутри функции.
type CallEdge struct {
	Caller string `json:"caller"`
	Callee string `json:"callee"`
	Line   int    `json:"line"`
}

// FileInfo — результат парсинга одного Go-файла.
type FileInfo struct {
	Path        string            `json:"path"`
	Package     string            `json:"package"`
	Imports     []string          `json:"imports,omitempty"`
	ImportNames map[string]string `json:"import_names,omitempty"`
	Symbols     []Symbol          `json:"symbols,omitempty"`
	Calls       []CallEdge        `json:"calls,omitempty"`
	ModTime     int64             `json:"mod_time"`
	Size        int64             `json:"size"`
	Hash        [16]byte          `json:"hash"`
	IsTest      bool              `json:"is_test"`
}

// parseFile парсит один Go-файл и извлекает символы, импорты, вызовы.
func parseFile(absPath, relPath string) (*FileInfo, error) {
	src, err := os.ReadFile(absPath)
	if err != nil {
		return nil, err
	}

	// Отдельный FileSet на файл, чтобы не копить память при инкрементальных refresh.
	fset := token.NewFileSet()

	f, err := parser.ParseFile(fset, absPath, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	info := &FileInfo{
		Path:        relPath,
		Package:     f.Name.Name,
		Hash:        md5.Sum(src),
		ImportNames: make(map[string]string),
		IsTest:      strings.HasSuffix(relPath, "_test.go"),
	}

	if fi, err := os.Stat(absPath); err == nil {
		info.ModTime = fi.ModTime().UnixNano()
		info.Size = fi.Size()
	}

	// Импорты и локальные имена.
	for _, imp := range f.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)
		info.Imports = append(info.Imports, importPath)

		name := ""
		if imp.Name != nil {
			name = imp.Name.Name
		} else {
			name = path.Base(importPath)
		}

		if name != "" && name != "_" && name != "." {
			info.ImportNames[name] = importPath
		}
	}

	// Декларации верхнего уровня.
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			sym := Symbol{
				Name:     d.Name.Name,
				Line:     fset.Position(d.Pos()).Line,
				Exported: d.Name.IsExported(),
				Doc:      firstDocLine(d.Doc),
			}

			if d.Recv != nil && len(d.Recv.List) > 0 {
				sym.Kind = KindMethod
				sym.Receiver = receiverTypeName(d.Recv.List[0].Type)
			} else {
				sym.Kind = KindFunc
			}

			info.Symbols = append(info.Symbols, sym)

			// Вызовы внутри тела.
			if d.Body != nil {
				callerName := sym.Name
				if sym.Receiver != "" {
					callerName = sym.Receiver + "." + sym.Name
				}

				ast.Inspect(d.Body, func(n ast.Node) bool {
					call, ok := n.(*ast.CallExpr)
					if !ok {
						return true
					}

					name := callName(call)
					if name != "" {
						info.Calls = append(info.Calls, CallEdge{
							Caller: callerName,
							Callee: name,
							Line:   fset.Position(call.Pos()).Line,
						})
					}

					return true
				})
			}

		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					kind := KindType
					switch s.Type.(type) {
					case *ast.StructType:
						kind = KindStruct
					case *ast.InterfaceType:
						kind = KindInterface
					}

					info.Symbols = append(info.Symbols, Symbol{
						Name:     s.Name.Name,
						Kind:     kind,
						Line:     fset.Position(s.Pos()).Line,
						Exported: s.Name.IsExported(),
						Doc:      firstDocLine(d.Doc),
					})

				case *ast.ValueSpec:
					kind := KindVar
					if d.Tok == token.CONST {
						kind = KindConst
					}

					for _, name := range s.Names {
						info.Symbols = append(info.Symbols, Symbol{
							Name:     name.Name,
							Kind:     kind,
							Line:     fset.Position(name.Pos()).Line,
							Exported: name.IsExported(),
						})
					}
				}
			}
		}
	}

	return info, nil
}

// callName извлекает имя вызываемой функции/метода.
//
// Мы намеренно поддерживаем только простые формы:
//   - Foo()
//   - pkg.Foo()
//   - obj.Foo(), если obj — идентификатор
//
// Сложные цепочки вроде s.repo.Find() пока не разрешаем,
// потому что без полноценного type checking это даёт много шума.
func callName(call *ast.CallExpr) string {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return fn.Name

	case *ast.SelectorExpr:
		if id, ok := fn.X.(*ast.Ident); ok {
			return id.Name + "." + fn.Sel.Name
		}
		return ""
	}

	return ""
}

// receiverTypeName извлекает имя типа из receiver.
func receiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return id.Name
		}
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return id.Name
		}
	}

	return ""
}

// firstDocLine возвращает первую строку doc-комментария.
func firstDocLine(doc *ast.CommentGroup) string {
	if doc == nil || len(doc.List) == 0 {
		return ""
	}

	text := strings.TrimSpace(doc.List[0].Text)
	text = strings.TrimPrefix(text, "//")
	text = strings.TrimPrefix(text, "/*")
	text = strings.TrimSuffix(text, "*/")
	text = strings.TrimSpace(text)

	if len(text) > 120 {
		text = textutil.LimitRunes(text, 120, "")
	}

	return text
}