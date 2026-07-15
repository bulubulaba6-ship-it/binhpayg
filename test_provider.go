package main

import (
	"fmt"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/util"
    "github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
    "github.com/router-for-me/CLIProxyAPI/v7/internal/config"
)

func main() {
    cfg, _ := config.LoadConfig("config.yaml")
    registry.InitFromConfig(cfg)
	fmt.Println(util.GetProviderName("gpt-5.5"))
}
