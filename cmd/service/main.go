package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os/exec"
	"path"
	"strings"
	"text/template"

	"github.com/gopcua/opcua/cmd/service/goname"
	"github.com/gopcua/opcua/errors"
)

var in, out, pkg string

func main() {
	log.SetFlags(0)

	flag.StringVar(&in, "in", "schema/Opc.Ua.Types.bsd", "Path to Opc.Ua.Types.bsd file")
	flag.StringVar(&out, "out", "ua", "Path to output directory")
	flag.StringVar(&pkg, "pkg", "ua", "Go package name")
	flag.Parse()

	dict, err := ReadTypes(in)
	if err != nil {
		log.Fatalf("Failed to read type definitions: %s", err)
	}

	writeEnums(Enums(dict))
	writeServiceRegister(ExtObjects(dict))
	writeExtObjects(ExtObjects(dict))
	writeRegisterExtObjects(ExtObjects(dict))
}

func writeEnums(enums []Type) {
	var b bytes.Buffer
	if err := FormatTypes(&b, enums); err != nil {
		log.Fatal(err)
	}
	write(b.Bytes(), path.Join(out, "enums_gen.go"))
}

func writeExtObjects(objs []Type) {
	var b bytes.Buffer
	if err := tmplReqResp.Execute(&b, nil); err != nil {
		log.Fatal(err)
	}
	if err := FormatTypes(&b, objs); err != nil {
		log.Fatal(err)
	}
	write(b.Bytes(), path.Join(out, "extobjs_gen.go"))
}

func writeRegisterExtObjects(objs []Type) {
	var b bytes.Buffer
	if err := tmplRegExtObjs.Execute(&b, objs); err != nil {
		log.Fatal(err)
	}
	write(b.Bytes(), path.Join(out, "register_extobjs_gen.go"))
}

func writeServiceRegister(objs []Type) {
	var b bytes.Buffer
	if err := tmplRegister.Execute(&b, objs); err != nil {
		log.Fatal(err)
	}
	write(b.Bytes(), path.Join(out, "service_gen.go"))
}

func write(src []byte, filename string) {
	var b bytes.Buffer
	if err := tmplHeader.Execute(&b, pkg); err != nil {
		log.Fatalf("Failed to generate header: %s", err)
	}

	b.Write(src)

	if err := ioutil.WriteFile(filename, b.Bytes(), 0644); err != nil {
		log.Fatalf("Failed to write %s: %v", filename, err)
	}

	if err := exec.Command("goimports", "-w", filename).Run(); err != nil {
		fmt.Println(string(src))
		log.Fatalf("Failed to format %s: %v", filename, err)
	}

	log.Printf("Wrote %s", filename)
}

var tmplHeader = template.Must(template.New("").Parse(`
// Copyright 2018-2020 opcua authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

// Code generated by cmd/service. DO NOT EDIT!

package {{.}}

import "time"

`))

func Enums(dict *TypeDictionary) []Type {
	var enums []Type
	for _, t := range dict.Enums {
		e := Type{
			Name: goname.Format(t.Name),
			Kind: KindEnum,
		}

		switch {
		case t.Bits <= 8:
			e.Type = "uint8"
		case t.Bits <= 16:
			e.Type = "uint16"
		case t.Bits <= 32:
			e.Type = "uint32"
		default:
			e.Type = "uint64"
		}

		for _, val := range t.Values {
			v := Value{
				Name:      goname.Format(e.Name + val.Name),
				ShortName: val.Name,
				Value:     val.Value,
			}
			e.Values = append(e.Values, v)
		}
		enums = append(enums, e)
	}
	return enums
}

func ExtObjects(dict *TypeDictionary) []Type {
	baseTypes := map[string]*Type{
		// Extensionobject is the base class for all extension objects.
		"ua:ExtensionObject": &Type{Name: "ExtensionObject"},

		// DataTypeDefinition is referenced in Opc.Ua.Types.bsd but not defined there
		// From what I can tell it is an abstract base class without any fields.
		// We define it here to be able to generate code for derived classes.
		"tns:DataTypeDefinition": &Type{Name: "DataTypeDefinition"},
	}

	var objects []Type
	for _, t := range dict.Types {
		// check if the base type is derived from ExtensionObject
		baseType := baseTypes[t.BaseType]
		if baseType == nil {
			continue
		}

		o := Type{
			Name: goname.Format(t.Name),
			Kind: KindExtensionObject,
			Base: baseType,
		}

		for _, f := range t.Fields {
			// skip fields containing the length of an array since
			// we create an array type
			if t.IsLengthField(f) {
				continue
			}

			of := Field{
				Name: goname.Format(f.Name),
				Type: goFieldType(f),
			}
			if of.Name == "AttributeID" {
				of.Type = "AttributeID"
			}
			o.Fields = append(o.Fields, of)
		}

		// register it as derived from ExtensionObject
		// we need to register it with target namespace 'tns:' since t.Name only contains the
		// base name.
		baseTypes["tns:"+t.Name] = &o

		objects = append(objects, o)
	}
	return objects
}

type Kind int

const (
	KindEnum Kind = iota
	KindExtensionObject
)

type Type struct {
	// Name is the Go name of the OPC/UA type.
	Name string

	// Type is the Go type of the OPC/UA type.
	Type string

	// Kind is the kind of OPC/UA type.
	Kind Kind

	// Base is the OPC/UA type this type is derived from.
	Base *Type

	// Fields is the list of struct fields.
	Fields []Field

	// Values is the list of enum values.
	Values []Value
}

func (t Type) IsRequest() bool {
	return len(t.Fields) > 0 && t.Fields[0].Type == "*RequestHeader"
}

func (t Type) IsResponse() bool {
	return len(t.Fields) > 0 && t.Fields[0].Type == "*ResponseHeader"
}

type Value struct {
	Name      string
	ShortName string
	Value     int
}

type Field struct {
	Name string
	Type string
}

func FormatTypes(w io.Writer, types []Type) error {
	for _, t := range types {
		if err := FormatType(w, t); err != nil {
			return err
		}
	}
	return nil
}

func FormatType(w io.Writer, t Type) error {
	switch t.Kind {
	case KindEnum:
		return tmplEnum.Execute(w, t)
	case KindExtensionObject:
		return tmplExtObject.Execute(w, t)
	default:
		return errors.Errorf("invalid type: %d", t.Kind)
	}
}

var tmplEnum = template.Must(template.New("").Parse(`
type {{.Name}} {{.Type}}

func {{.Name}}FromString(s string) {{.Name}} {
	switch s {
		{{range $i, $v := .Values}}case "{{.ShortName}}": return {{$v.Value}}
		{{end}}default:
		return 0
	}
}

const (
	{{$Name := .Name}}
	{{range $i, $v := .Values}}{{$v.Name}} {{$Name}} = {{$v.Value}}
	{{end}}
)
`))

var tmplRegExtObjs = template.Must(template.New("").Parse(`
import (
	"github.com/gopcua/opcua/id"
)

func init() {
	{{- range $i, $v := . -}}
		RegisterExtensionObject(NewNumericNodeID(0, id.{{$v.Name}}_Encoding_DefaultBinary), new({{$v.Name}}))
	{{end -}}
}
`))

var tmplReqResp = template.Must(template.New("").Parse(`
type Request interface {
	Header() *RequestHeader
	SetHeader(*RequestHeader)
}

type Response interface {
	Header() *ResponseHeader
	SetHeader(*ResponseHeader)
}
`))

var tmplExtObject = template.Must(template.New("").Parse(`
type {{.Name}} struct {
	{{- if .Fields}}
		{{range $i, $v := .Fields}}{{$v.Name}} {{$v.Type}}
		{{end}}
	{{end -}}
}
{{- if .IsRequest}}

func (t *{{.Name}}) Header() *RequestHeader {
	return t.RequestHeader
}

func (t *{{.Name}}) SetHeader(h *RequestHeader) {
	t.RequestHeader = h
}
{{- end}}
{{- if .IsResponse}}

func (t *{{.Name}}) Header() *ResponseHeader {
	return t.ResponseHeader
}

func (t *{{.Name}}) SetHeader(h *ResponseHeader) {
	t.ResponseHeader = h
}
{{- end}}
`))

var funcs = template.FuncMap{
	"isService": func(s string) bool {
		return strings.HasSuffix(s, "Request") || strings.HasSuffix(s, "Response") || s == "ServiceFault"
	},
}

var tmplRegister = template.Must(template.New("").Funcs(funcs).Parse(`

import "github.com/gopcua/opcua/id"

func init() {
	{{- range $i, $v := . -}}
		{{- if isService $v.Name -}}
			RegisterService(id.{{$v.Name}}_Encoding_DefaultBinary, new({{$v.Name}}))
		{{end -}}
	{{end -}}
}
`))

var builtins = map[string]string{
	"opc:Boolean":    "bool",
	"opc:Byte":       "uint8",
	"opc:SByte":      "int8",
	"opc:Int16":      "int16",
	"opc:Int32":      "int32",
	"opc:Int64":      "int64",
	"opc:UInt16":     "uint16",
	"opc:UInt32":     "uint32",
	"opc:UInt64":     "uint64",
	"opc:Float":      "float32",
	"opc:Double":     "float64",
	"opc:String":     "string",
	"opc:DateTime":   "time.Time",
	"opc:ByteString": "[]byte",
	"ua:StatusCode":  "StatusCode",
	"opc:Guid":       "*GUID",
}

func goFieldType(f *StructField) string {
	t, builtin := builtins[f.Type]
	if t == "" {
		prefix := strings.NewReplacer("ua:", "", "tns:", "")
		t = goname.Format(prefix.Replace(f.Type))
	}
	if !f.IsEnum && !builtin {
		t = "*" + t
	}
	if f.IsSlice() {
		t = "[]" + t
	}
	return t
}