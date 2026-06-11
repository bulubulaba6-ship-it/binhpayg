package main

import (
	"fmt"
)

func main() {
    var m map[string]interface{}
    v, ok := m["test"].(float64)
    fmt.Printf("v=%v ok=%v\n", v, ok)
}
