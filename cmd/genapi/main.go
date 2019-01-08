package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"log"
	"os"
	"os/user"
	"path"
	"strings"
	"text/template"
)

const genmsg = "// Code generated by genapi, which is a API generation tool for Gorgonia. DO NOT EDIT."

const (
	apigenOut = "api_gen.go"
	unaryOps  = "operatorPointwise_unary_const.go"
	binaryOps = "operatorPointwise_binary_const.go"
)

var (
	gopath, gorgonialoc string
)

var funcmap = template.FuncMap{
	"lower": strings.ToLower,
}

var (
	unaryTemplate  *template.Template
	binaryTemplate *template.Template
)

const unaryTemplateRaw = ` // {{.FnName}} performs a pointwise {{lower .FnName}}.
func {{.FnName}}(a *Node) (*Node, error) { return unaryOpNode(newElemUnaryOp({{.OpType}}, a), a) }
`

const binaryTemplateRaw = `// {{.FnName}} perfors a pointwise {{lower .FnName}} operation.
{{if .AsSame -}}//	retSame indicates if the data type of the return value should be the same as the input data type. It defaults to Bool otherwise.
{{end -}}
func {{.FnName}}(a, b *Node{{if .AsSame}}, retSame bool{{end}}) (*Node, error) { {{if not .AsSame -}}return binOpNode(newElemBinOp({{.OpType}}, a, b), a, b) {{else -}}
	op := newElemBinOp({{.OpType}}, a, b)
	op.retSame = retSame
	return binOpNode(op, a, b)
{{end -}}
}

// {{.FnName}}Op ...
func New{{.FnName}}Operation() Operation {
	return func(g graph.WeightedDirected, n node.Node) (ops.Op, error) {
		it := getOrderedChildren(g, n)
		children := make([]*Node, it.Len())
		for i := 0; it.Next(); i++ {
			children[i] = it.Node().(*Node)
		}
		return newElemBinOp({{.OpType}}, children[0], children[1]), nil
	}
}
`

func init() {
	gopath = os.Getenv("GOPATH")
	// now that go can have a default gopath, this checks that path
	if gopath == "" {
		usr, err := user.Current()
		if err != nil {
			log.Fatal(err)
		}
		gopath = path.Join(usr.HomeDir, "go")
		stat, err := os.Stat(gopath)
		if err != nil {
			log.Fatal(err)
		}
		if !stat.IsDir() {
			log.Fatal("You need to define a $GOPATH")
		}
	}
	gorgonialoc = path.Join(gopath, "src/gorgonia.org/gorgonia")
	unaryTemplate = template.Must(template.New("Unary").Funcs(funcmap).Parse(unaryTemplateRaw))
	binaryTemplate = template.Must(template.New("Binary").Funcs(funcmap).Parse(binaryTemplateRaw))
}

func generateUnary(outFile io.Writer) {
	// parse operator_unary_const.go
	filename := path.Join(gorgonialoc, unaryOps)
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, nil, parser.AllErrors)
	if err != nil {
		log.Fatal(err)
	}

	unaryNames := constTypes(file.Decls, "ʘUnaryOperatorType", "maxʘUnaryOperator")
	for _, v := range unaryNames {
		apiName := strings.Title(strings.TrimSuffix(v, "OpType"))
		// legacy issue
		if apiName == "Ln" {
			apiName = "Log"
		}
		data := struct{ FnName, OpType string }{apiName, v}
		unaryTemplate.Execute(outFile, data)
	}

}
func generateBinary(outFile io.Writer) {
	// parse operator_binary_const.go
	filename := path.Join(gorgonialoc, binaryOps)
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, nil, parser.AllErrors)
	if err != nil {
		log.Fatal(err)
	}

	binaryNames := constTypes(file.Decls, "ʘBinaryOperatorType", "maxʘBinaryOpType")
	log.Printf("%v", binaryNames)
	for _, v := range binaryNames {
		apiName := strings.Title(strings.TrimSuffix(v, "OpType"))
		// legacy issue
		switch apiName {
		case "Mul":
			apiName = "HadamardProd"
		case "Div":
			apiName = "HadamardDiv"
		}
		data := struct {
			FnName, OpType string
			AsSame         bool
		}{apiName, v, false}
		switch apiName {
		case "Lt", "Gt", "Lte", "Gte", "Eq", "Ne":
			data.AsSame = true
		}
		binaryTemplate.Execute(outFile, data)
	}
}

func constTypes(decls []ast.Decl, accept, max string) (names []string) {
	for i, decl := range decls {
		log.Printf("DECL %d: %T", i, decl)
		switch d := decl.(type) {
		case *ast.GenDecl:
			if d.Tok.IsKeyword() && d.Tok.String() == "const" {
				log.Printf("\t%v", d.Tok.String())

				// get the type
				if len(d.Specs) == 0 {
					continue
				}

				var typename string
				typ := d.Specs[0].(*ast.ValueSpec).Type
				if typ == nil {
					continue
				}
				if id, ok := typ.(*ast.Ident); ok {
					typename = id.Name
				}
				if typename != accept {
					continue
				}

				for _, spec := range d.Specs {
					name := spec.(*ast.ValueSpec).Names[0].Name
					if name == max {
						continue
					}
					names = append(names, name)
				}
			}
		default:
		}
	}
	return
}

func main() {
	outFileName := path.Join(gorgonialoc, apigenOut)
	outFile, err := os.OpenFile(outFileName, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer outFile.Close()
	fmt.Fprintf(outFile, `package gorgonia 

import (
	"gonum.org/v1/gonum/graph"
	"gorgonia.org/gorgonia/node"
	"gorgonia.org/gorgonia/ops"
)

%v

`, genmsg)

	generateUnary(outFile)
	generateBinary(outFile)
}
