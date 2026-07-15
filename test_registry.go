package main

import (
	"fmt"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
)

func main() {
    registry.GetGlobalRegistry().RegisterClient("client1", "openai", []*registry.ModelInfo{
        {ID: "gpt-5.5", Object: "model"},
    })
    providers := registry.GetGlobalRegistry().GetModelProviders("gpt-5.5")
    fmt.Println(providers)
}
