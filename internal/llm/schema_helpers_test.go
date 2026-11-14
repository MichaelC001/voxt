package llm

// More schema builders, as Object and String: only the tests use them.

func Array(items *Schema) *Schema   { return &Schema{Type: "array", Items: items} }
func Number() *Schema               { return &Schema{Type: "number"} }
func Integer() *Schema              { return &Schema{Type: "integer"} }
func Boolean() *Schema              { return &Schema{Type: "boolean"} }
func Enum(values ...string) *Schema { return &Schema{Type: "string", Enum: values} }
