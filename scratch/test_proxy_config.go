package main

import (
	"fmt"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

func main() {
	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		fmt.Printf("❌ Lỗi load config: %v\n", err)
		return
	}

	fmt.Printf("✅ Cấu hình Strategy: %s\n", cfg.Routing.Strategy)
	fmt.Printf("✅ Upstream Proxy (Routing): %s\n", cfg.Routing.UpstreamProxy)
	fmt.Printf("🚀 Thực tế ProxyURL sử dụng: %s\n", cfg.ProxyURL)

	if cfg.ProxyURL == "socks5h://127.0.0.1:1080" {
		fmt.Println("\n💎 KẾT QUẢ: Code đã nhận diện Proxy chính xác! Sẵn sàng kết nối qua Wireguard.")
	} else {
		fmt.Println("\n⚠️ CẢNH BÁO: ProxyURL chưa được đồng bộ đúng. Cần kiểm tra lại logic.")
	}
}
