# schema

Package `schema` generates JSON Schema from Go struct types using reflection.

## Usage

Define an input struct with `json` and `desc` tags:

```go
type askInput struct {
    Question string   `json:"question" desc:"The question to ask"`
    Options  []string `json:"options,omitempty" desc:"Optional choices"`
}
```

Generate the schema:

```go
InputSchema: schema.Generate[askInput]()
// → {"type":"object","properties":{"question":{"type":"string","description":"The question to ask"},"options":{"type":"array","items":{"type":"string"},"description":"Optional choices"}},"required":["question"]}
```

## Tag conventions

- **`json:"name"`** — field name in the schema; fields without `omitempty` are added to `required`
- **`json:"name,omitempty"`** — optional field (not in `required`)
- **`desc:"..."`** — JSON Schema `description` for the field

## Supported types

| Go type | JSON Schema type |
|---------|-----------------|
| `string` | `"string"` |
| `int`, `int64`, etc. | `"integer"` |
| `float64` | `"number"` |
| `bool` | `"boolean"` |
| `[]string` | `{"type":"array","items":{"type":"string"}}` |
| `[]struct{...}` | `{"type":"array","items":{"type":"object",...}}` |
| `map[string]string` | `{"type":"object","additionalProperties":{"type":"string"}}` |
| `struct{...}` | `{"type":"object","properties":{...}}` |
