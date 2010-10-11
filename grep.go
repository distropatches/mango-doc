package main

import (
	"go/ast"
	"go/token"
	"go/doc"
	"container/vector"
	"sort"
	"strings"
)

func lit(x interface{}) []byte {
	if b, ok := x.(*ast.BasicLit); ok {
		v := b.Value
		switch b.Kind {
		case token.CHAR, token.STRING:
			//need to copy so sentences doesn't muss it up
			nv := make([]byte, len(v)-2)
			copy(nv, v[1:len(v)-1])
			v = nv
		}
		return v
	}
	return nil
}

func grep_version(pkg *ast.Package) string {
	for _, file := range pkg.Files {
		for _, decl := range file.Decls {
			if g, ok := decl.(*ast.GenDecl); ok {
				if g.Tok == token.CONST {
					for _, s := range g.Specs {
						if v, ok := s.(*ast.ValueSpec); ok {
							for i, n := range v.Names {
								if n.Name == "Version" {
									t := v.Values[i]
									if b, ok := t.(*ast.BasicLit); ok {
										return string(lit(b))
									}
								}
							}
						}
					}
				}
			}
		}
	}
	return ""
}

func flatten(docs *doc.PackageDoc) <-chan string {
	out := make(chan string)
	var sub func(interface{})
	sub = func(x interface{}) {
		switch t := x.(type) {
		case []*doc.ValueDoc:
			for _, v := range t {
				out <- v.Doc
			}
		case []*doc.FuncDoc:
			for _, v := range t {
				out <- v.Doc
			}
		case []*doc.TypeDoc:
			for _, v := range t {
				out <- v.Doc
				sub(v.Consts)
				sub(v.Vars)
				sub(v.Factories)
				sub(v.Methods)
			}
		}
	}
	go func() {
		out <- docs.Doc
		sub(docs.Consts)
		sub(docs.Types)
		sub(docs.Vars)
		sub(docs.Funcs)
		close(out)
	}()
	return out
}

var refrx = RX(SP + rrx)

func (m *M) find_refs() {
	var acc vector.StringVector
	seen := map[string]bool{}
	seen[m.name+"("+m.sec+")"] = true //don't want recursive references
	for str := range flatten(m.docs) {
		for _, idx := range refrx.FindAllStringIndex(str, -1) {
			sub := strings.TrimSpace(str[idx[0]:idx[1]])
			switch sub[len(sub)-2] { //check the part in the ()
			case '1', '2', '3', '4', '5', '6', '7', '8', '9', '0',
				'n', 'o', 'l', 'x', 'p':
				//okay, even though most of these are unlikely
				//and some deprecated
			default:
				continue
			}
			if !seen[sub] {
				seen[sub] = true
				acc.Push(sub)
			}
		}
	}
	sort.Sort(&acc)
	m.refs = []string(acc)
}