package main

import (
	"fmt"
	"github.com/tidwall/sjson"
)

func main() {
	json := `{"messages": [{"role": "user", "content": "Hello"}, {"role": "assistant", "content": "Hi"}]}`
	
	// Test index 0
	val1, _ := sjson.Set(json, "messages.0", "OVERWRITE")
	fmt.Println("Index 0:", val1)
}
