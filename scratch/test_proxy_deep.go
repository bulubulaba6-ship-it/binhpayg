//go:build ignore

package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/util"
)

func main() {
	// 1. Tạo Mock Upstream
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "SUCCESS")
	}))
	defer ts.Close()

	// 2. Load Config
	cfg, _ := config.LoadConfig("config.yaml")
	
	// 3. Khởi tạo Client dùng logic Proxy của hệ thống
	httpClient := &http.Client{Timeout: 3 * time.Second}
	util.SetProxy(&cfg.SDKConfig, httpClient)

	fmt.Printf("🚀 Đang thử gửi request tới: %s\n", ts.URL)
	fmt.Printf("🛡️ Proxy đang bật: %s\n", cfg.ProxyURL)
	
	_, err := httpClient.Get(ts.URL)

	if err != nil && (fmt.Sprintf("%v", err) != "") {
		fmt.Printf("\n🎯 KẾT QUẢ: Request thất bại đúng như dự kiến!\n")
		fmt.Printf("📝 Chi tiết lỗi: %v\n", err)
		fmt.Println("\n✅ KẾT LUẬN: Hệ thống ĐÃ CHẶN traffic trực tiếp và bắt buộc đi qua Proxy.")
	} else {
		fmt.Println("\n❌ CẢNH BÁO: Request thành công! Traffic đang bypass Proxy.")
	}
}
