# FINK Orchestrator Core

CLI tool để cài đặt FINK Core Engine và cấu hình Enterprise tự động.

## Cài đặt

Sử dụng **bunx**:
```bash
bunx fink-claude-code-installer
```
Hoặc **npx**:
```bash
npx fink-claude-code-installer
```
*Lưu ý: Tool sẽ yêu cầu bạn nhập Auth Token khi chạy. Mặc định sử dụng tiếng Việt.*

## Tùy chọn Ngôn ngữ

- **Tiếng Việt (Mặc định)**: `-vi`
- **Tiếng Anh (International)**: `-en`

Ví dụ chạy với tiếng Anh:
```bash
npx fink-claude-code-installer -en
```

## Tính năng

- **Tự động cài đặt**: Luôn cài đặt Claude Code phiên bản mới nhất từ registry.
- **Subagent-First Architecture**: Chuyển các công cụ nặng (Search, Indexing, Memory) vào các agent chuyên biệt như `@web-researcher`, `@code-analyst`, `@memory-keeper` để tiết kiệm ~80K tokens mỗi tin nhắn.
- **Cấu hình Enterprise**: Thiết lập sẵn API endpoint ổn định và bảo mật.
- **Tối ưu hóa Model**: Sử dụng chế độ tự động hóa cấu hình tốt nhất cho mọi model.
- **An toàn**: Thiết lập biến môi trường hệ thống một cách an toàn và tinh gọn.
- **Fink Optimization Pack (PRO only)**: Tự động cài đặt skills, rules và agents chuyên sâu cho lập trình viên.

## Cấu hình

Tool sẽ tự động thiết lập đè cấu hình tại `~/.claude/settings.json` với Base URL và Auth Token tương ứng.

## Yêu cầu hệ thống

Để công cụ sẳn sàng và chạy tốt, máy tính của bạn **bắt buộc** cần cài đặt:
1. **Node.js**: Phiên bản mới nhất (LTS) để thực thi lệnh npx/npm.
2. **Git**: Để tải và đồng bộ các gói tối ưu hóa (Fink Optimization Pack).

---
**Keywords**: `fink` `orchestrator` `gateway` `cli` `installer`
