package main

import (
	"fmt"
	"go/types"
	"path/filepath"
	"reflect"
	"strings"

	"golang.org/x/tools/go/packages"
)

type generatedEntField struct {
	field    *types.Var
	jsonName string
}

// ResolveEntSchemas binds schema fields to the exact names and method
// signatures in an entc-generated package. Generated code is the source of
// truth because entc naming can be extended with custom acronyms.
func ResolveEntSchemas(schemaDir, importPath string, schemas []EntSchema) ([]EntSchema, error) {
	absSchemaDir, err := filepath.Abs(schemaDir)
	if err != nil {
		return nil, fmt.Errorf("resolve Ent schema dir %q: %w", schemaDir, err)
	}
	pkgs, err := packages.Load(&packages.Config{
		Dir:  absSchemaDir,
		Mode: packages.NeedName | packages.NeedTypes | packages.NeedTypesSizes | packages.NeedImports | packages.NeedDeps,
	}, importPath)
	if err != nil {
		return nil, fmt.Errorf("load generated Ent package %q: %w", importPath, err)
	}
	if len(pkgs) != 1 {
		return nil, fmt.Errorf("load generated Ent package %q: got %d packages, want 1", importPath, len(pkgs))
	}
	pkg := pkgs[0]
	if len(pkg.Errors) > 0 {
		return nil, fmt.Errorf("load generated Ent package %q: %w", importPath, pkg.Errors[0])
	}
	if pkg.Types == nil {
		return nil, fmt.Errorf("load generated Ent package %q: type information is unavailable", importPath)
	}

	resolved := cloneEntSchemas(schemas)
	for i := range resolved {
		if err := resolveEntSchemaSignatures(pkg.Types, &resolved[i]); err != nil {
			return nil, fmt.Errorf("resolve generated Ent schema %q: %w", resolved[i].Name, err)
		}
	}
	if err := validateEntSchemas(resolved); err != nil {
		return nil, fmt.Errorf("validate resolved Ent schemas: %w", err)
	}
	return resolved, nil
}

func cloneEntSchemas(schemas []EntSchema) []EntSchema {
	cloned := make([]EntSchema, len(schemas))
	for i, schema := range schemas {
		cloned[i] = schema
		cloned[i].Fields = append([]EntField(nil), schema.Fields...)
		cloned[i].Edges = append([]EntEdge(nil), schema.Edges...)
	}
	return cloned
}

func resolveEntSchemaSignatures(pkg *types.Package, schema *EntSchema) error {
	entity, entityStruct, err := lookupEntStruct(pkg, schema.Name)
	if err != nil {
		return err
	}
	create, _, err := lookupEntStruct(pkg, schema.Name+"Create")
	if err != nil {
		return err
	}
	client, _, err := lookupEntStruct(pkg, schema.Name+"Client")
	if err != nil {
		return err
	}

	generatedFields, mixinFields, err := entFieldsBySchemaName(entityStruct, schema.Fields)
	if err != nil {
		return err
	}
	for i := range schema.Fields {
		field := &schema.Fields[i]
		generated, ok := generatedFields[field.Name]
		if !ok {
			return fmt.Errorf("field %q is absent; regenerate Ent before running seedling-gen", field.Name)
		}
		if !field.CustomGoType && !entSchemaTypeMatches(field.Type, field.Nillable, generated.Type()) {
			return fmt.Errorf("field %q generated type is %s, incompatible with schema field.%s; regenerate Ent before running seedling-gen", field.Name, entDisplayType(generated.Type()), field.Type)
		}
		resolveEntFieldType(field, generated)
		if err := resolveEntFieldSetter(create, field, generated.Type()); err != nil {
			return fmt.Errorf("field %q: %w", field.Name, err)
		}
	}
	for _, generated := range mixinFields {
		field, err := resolveEntMixinField(create, generated)
		if err != nil {
			return err
		}
		schema.Fields = append(schema.Fields, field)
	}

	idField := entStructFieldByName(entityStruct, "ID")
	if idField == nil {
		return fmt.Errorf("entity %s has no ID field", schema.Name)
	}
	contextType, err := lookupEntImportedType(pkg, "context", "Context")
	if err != nil {
		return err
	}
	errorType := types.Universe.Lookup("error").Type()

	createMethod, err := validateEntMethod(client, "Create", nil)
	if err != nil {
		return err
	}
	if err := validateEntResults(client, "Create", createMethod, []types.Type{types.NewPointer(create)}); err != nil {
		return err
	}
	saveMethod, err := validateEntMethod(create, "Save", []types.Type{contextType})
	if err != nil {
		return err
	}
	if err := validateEntResults(create, "Save", saveMethod, []types.Type{types.NewPointer(entity), errorType}); err != nil {
		return err
	}

	deleteMethod, err := validateEntMethod(client, "DeleteOneID", []types.Type{idField.Type()})
	if err != nil {
		return err
	}
	if deleteMethod.Results().Len() != 1 {
		return fmt.Errorf("generated method %s.DeleteOneID has %d results, want 1", client.Obj().Name(), deleteMethod.Results().Len())
	}
	deletePointer, ok := types.Unalias(deleteMethod.Results().At(0).Type()).(*types.Pointer)
	if !ok {
		return fmt.Errorf("generated method %s.DeleteOneID result is %s, want pointer to delete builder", client.Obj().Name(), deleteMethod.Results().At(0).Type())
	}
	deleteBuilder, ok := types.Unalias(deletePointer.Elem()).(*types.Named)
	if !ok {
		return fmt.Errorf("generated method %s.DeleteOneID result is %s, want pointer to named delete builder", client.Obj().Name(), deleteMethod.Results().At(0).Type())
	}
	execMethod, err := validateEntMethod(deleteBuilder, "Exec", []types.Type{contextType})
	if err != nil {
		return err
	}
	if err := validateEntResults(deleteBuilder, "Exec", execMethod, []types.Type{errorType}); err != nil {
		return err
	}
	return nil
}

func lookupEntImportedType(pkg *types.Package, importPath, name string) (types.Type, error) {
	for _, imported := range pkg.Imports() {
		if imported.Path() != importPath {
			continue
		}
		object := imported.Scope().Lookup(name)
		if object == nil {
			break
		}
		return object.Type(), nil
	}
	return nil, fmt.Errorf("generated package does not import %s.%s", importPath, name)
}

func entStructFieldByName(entity *types.Struct, name string) *types.Var {
	for field := range entity.Fields() {
		if field.Name() == name {
			return field
		}
	}
	return nil
}

func lookupEntStruct(pkg *types.Package, name string) (*types.Named, *types.Struct, error) {
	obj := pkg.Scope().Lookup(name)
	if obj == nil {
		return nil, nil, fmt.Errorf("generated type %s.%s is absent; regenerate Ent before running seedling-gen", pkg.Name(), name)
	}
	typeName, ok := obj.(*types.TypeName)
	if !ok {
		return nil, nil, fmt.Errorf("generated object %s.%s is not a type", pkg.Name(), name)
	}
	named, ok := types.Unalias(typeName.Type()).(*types.Named)
	if !ok {
		return nil, nil, fmt.Errorf("generated type %s.%s is not named", pkg.Name(), name)
	}
	underlying, ok := named.Underlying().(*types.Struct)
	if !ok {
		return nil, nil, fmt.Errorf("generated type %s.%s is not a struct", pkg.Name(), name)
	}
	return named, underlying, nil
}

func resolveEntFieldType(field *EntField, generated *types.Var) {
	field.GoName = generated.Name()
	field.GoType = entDisplayType(generated.Type())
	field.DefaultType = entDefaultType(generated.Type())
}

func entDisplayType(value types.Type) string {
	return types.TypeString(value, func(pkg *types.Package) string {
		return pkg.Name()
	})
}

func entDefaultType(value types.Type) string {
	value = types.Unalias(value)
	if _, ok := value.(*types.Pointer); ok {
		return ""
	}
	if named, ok := value.(*types.Named); ok {
		if object := named.Obj(); object.Pkg() != nil && object.Pkg().Path() == "time" && object.Name() == "Time" {
			return "time.Time"
		}
		value = types.Unalias(named.Underlying())
	}
	if basic, ok := value.(*types.Basic); ok {
		defaultTypes := map[types.BasicKind]string{
			types.String:  "string",
			types.Bool:    "bool",
			types.Int:     "int",
			types.Int8:    "int8",
			types.Int16:   "int16",
			types.Int32:   "int32",
			types.Int64:   "int64",
			types.Uint:    "uint",
			types.Uint8:   "uint8",
			types.Uint16:  "uint16",
			types.Uint32:  "uint32",
			types.Uint64:  "uint64",
			types.Float32: "float32",
			types.Float64: "float64",
		}
		return defaultTypes[basic.Kind()]
	}
	slice, ok := value.(*types.Slice)
	if !ok {
		return ""
	}
	element, ok := types.Unalias(slice.Elem()).(*types.Basic)
	if ok && element.Kind() == types.Uint8 {
		return "[]byte"
	}
	return ""
}

func entSchemaTypeMatches(schemaType string, nillable bool, actual types.Type) bool {
	actual = types.Unalias(actual)
	if nillable {
		pointer, ok := actual.(*types.Pointer)
		if !ok {
			return false
		}
		actual = types.Unalias(pointer.Elem())
	} else if _, ok := actual.(*types.Pointer); ok {
		return false
	}

	underlying := actual
	if named, ok := actual.(*types.Named); ok {
		if schemaType == "Time" {
			object := named.Obj()
			return object.Pkg() != nil && object.Pkg().Path() == "time" && object.Name() == "Time"
		}
		underlying = types.Unalias(named.Underlying())
	}

	if basic, ok := underlying.(*types.Basic); ok {
		want := map[string]types.BasicKind{
			"String":  types.String,
			"Bool":    types.Bool,
			"Int":     types.Int,
			"Int8":    types.Int8,
			"Int16":   types.Int16,
			"Int32":   types.Int32,
			"Int64":   types.Int64,
			"Uint":    types.Uint,
			"Uint8":   types.Uint8,
			"Uint16":  types.Uint16,
			"Uint32":  types.Uint32,
			"Uint64":  types.Uint64,
			"Float32": types.Float32,
			"Float64": types.Float64,
			"Enum":    types.String,
		}
		if kind, ok := want[schemaType]; ok {
			return basic.Kind() == kind
		}
		return schemaType != "Time" && schemaType != "Bytes"
	}
	if schemaType == "Bytes" {
		slice, ok := underlying.(*types.Slice)
		if !ok {
			return false
		}
		element, ok := types.Unalias(slice.Elem()).(*types.Basic)
		return ok && element.Kind() == types.Uint8
	}

	// UUID, JSON, and custom Ent field constructors intentionally accept
	// application-defined Go types that cannot be inferred from the AST alone.
	return schemaType == "UUID" || schemaType == "JSON" || schemaType == "Other" || schemaType == ""
}

func resolveEntMixinField(create *types.Named, generated generatedEntField) (EntField, error) {
	goName := generated.field.Name()
	name := generated.jsonName
	if name == "" || name == "-" {
		name = toSnakeCase(goName)
	}
	field := EntField{
		Name:        name,
		JSONName:    generated.jsonName,
		Optional:    false,
		FromMixin:   true,
		DefaultType: entDefaultType(generated.field.Type()),
	}
	resolveEntFieldType(&field, generated.field)
	if _, ok := types.Unalias(generated.field.Type()).(*types.Pointer); ok {
		field.Nillable = true
	}
	if err := resolveEntFieldSetter(create, &field, generated.field.Type()); err != nil {
		return EntField{}, fmt.Errorf("generated mixin field %q: %w", goName, err)
	}
	return field, nil
}

func resolveEntFieldSetter(create *types.Named, field *EntField, entityType types.Type) error {
	setName := "Set" + field.GoName
	setNillableName := "SetNillable" + field.GoName
	if field.Nillable && entMethodAccepts(create, setNillableName, entityType) {
		field.SetterName = setNillableName
		return nil
	}
	if entMethodAccepts(create, setName, entityType) {
		field.SetterName = setName
		return nil
	}
	pointer, ok := types.Unalias(entityType).(*types.Pointer)
	if ok && entMethodAccepts(create, setName, pointer.Elem()) {
		field.SetterName = setName
		field.SetterDeref = true
		return nil
	}
	// A setter exists but took none of the branches above, so its parameter is
	// incompatible. Report that specific mismatch rather than the generic
	// message; validateEntMethod cannot succeed here, because it checks exactly
	// what entMethodAccepts already rejected.
	for _, name := range []string{setName, setNillableName} {
		if types.NewMethodSet(types.NewPointer(create)).Lookup(nil, name) == nil {
			continue
		}
		if err := mustFailEntMethodCheck(create, name, entityType); err != nil {
			return err
		}
	}
	return fmt.Errorf("generated field %s (%s) has no compatible Set or SetNillable method", field.GoName, entityType)
}

// mustFailEntMethodCheck describes why a setter that exists cannot take the
// entity's field type. It never returns nil for a setter entMethodAccepts
// rejected, but guards against that anyway so a silently unset SetterName can
// never reach the templates.
func mustFailEntMethodCheck(create *types.Named, name string, entityType types.Type) error {
	if _, err := validateEntMethod(create, name, []types.Type{entityType}); err != nil {
		return err
	}
	return fmt.Errorf("generated method %s.%s accepts %s but was not selected as a setter", create.Obj().Name(), name, entityType)
}

func entMethodAccepts(receiver *types.Named, name string, param types.Type) bool {
	selection := types.NewMethodSet(types.NewPointer(receiver)).Lookup(nil, name)
	if selection == nil {
		return false
	}
	signature, ok := selection.Obj().Type().(*types.Signature)
	return ok && signature.Params().Len() == 1 && types.Identical(signature.Params().At(0).Type(), param)
}

func entFieldsBySchemaName(entity *types.Struct, schemaFields []EntField) (map[string]*types.Var, []generatedEntField, error) {
	hasExplicitID := false
	for _, field := range schemaFields {
		if field.Name == "id" {
			hasExplicitID = true
			break
		}
	}

	generated := make([]generatedEntField, 0, entity.NumFields())
	for i := range entity.NumFields() {
		field := entity.Field(i)
		if !field.Exported() || field.Name() == "Edges" || field.Name() == "ID" && !hasExplicitID {
			continue
		}
		jsonName := strings.Split(reflect.StructTag(entity.Tag(i)).Get("json"), ",")[0]
		generated = append(generated, generatedEntField{field: field, jsonName: jsonName})
	}

	fields := make(map[string]*types.Var, len(schemaFields))
	used := make([]bool, len(generated))
	for _, field := range schemaFields {
		jsonName := field.JSONName
		if jsonName == "" {
			jsonName = field.Name
		}
		matches := make([]int, 0, 1)
		for j, candidate := range generated {
			if used[j] || candidate.jsonName != jsonName {
				continue
			}
			matches = append(matches, j)
		}
		if len(matches) != 1 {
			matches = matches[:0]
			for j, candidate := range generated {
				if !used[j] && normalizeSQLCIdentifier(candidate.field.Name()) == normalizeSQLCIdentifier(field.Name) {
					matches = append(matches, j)
				}
			}
		}
		if len(matches) == 0 {
			return nil, nil, fmt.Errorf("field %q with JSON name %q is absent; regenerate Ent before running seedling-gen", field.Name, jsonName)
		}
		if len(matches) > 1 {
			return nil, nil, fmt.Errorf("field %q matches multiple generated fields; use an unambiguous JSON struct tag", field.Name)
		}
		match := matches[0]
		used[match] = true
		fields[field.Name] = generated[match].field
	}

	remaining := make([]generatedEntField, 0, len(generated)-len(fields))
	for i, candidate := range generated {
		if !used[i] {
			remaining = append(remaining, candidate)
		}
	}
	return fields, remaining, nil
}

func validateEntMethod(receiver *types.Named, name string, params []types.Type) (*types.Signature, error) {
	selection := types.NewMethodSet(types.NewPointer(receiver)).Lookup(nil, name)
	if selection == nil {
		return nil, fmt.Errorf("generated method %s.%s is absent; regenerate Ent before running seedling-gen", receiver.Obj().Name(), name)
	}
	signature, ok := selection.Obj().Type().(*types.Signature)
	if !ok {
		return nil, fmt.Errorf("generated object %s.%s is not a method", receiver.Obj().Name(), name)
	}
	if signature.Params().Len() != len(params) {
		return nil, fmt.Errorf("generated method %s.%s has %d parameters, want %d", receiver.Obj().Name(), name, signature.Params().Len(), len(params))
	}
	for i, expected := range params {
		actual := signature.Params().At(i).Type()
		if !types.Identical(actual, expected) {
			return nil, fmt.Errorf("generated method %s.%s parameter %d has type %s, want %s", receiver.Obj().Name(), name, i, actual, expected)
		}
	}
	return signature, nil
}

func validateEntResults(receiver *types.Named, name string, signature *types.Signature, results []types.Type) error {
	if signature.Results().Len() != len(results) {
		return fmt.Errorf("generated method %s.%s has %d results, want %d", receiver.Obj().Name(), name, signature.Results().Len(), len(results))
	}
	for i, expected := range results {
		actual := signature.Results().At(i).Type()
		if !types.Identical(actual, expected) {
			return fmt.Errorf("generated method %s.%s result %d has type %s, want %s", receiver.Obj().Name(), name, i, actual, expected)
		}
	}
	return nil
}
