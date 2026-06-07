# **Hướng dẫn Cài đặt Fink Router**

Fink Router là dịch vụ API Gateway cho phép truy cập các mô hình trí tuệ nhân tạo hàng đầu như Claude (Haiku, Sonnet, Opus 4.8), ChatGPT và Cursor thông qua một tài khoản duy nhất. Dịch vụ hỗ trợ thanh toán bằng VND (qua ví MOMO hoặc chuyển khoản ngân hàng) giúp tối ưu hóa chi phí sử dụng từ 5 đến 10 lần so với đăng ký trực tiếp.

* Trang chủ: [https://finkrouter.io.vn](https://finkrouter.io.vn/)  
* API Endpoint: [https://api.finkrouter.io.vn/](https://api.finkrouter.io.vn/)   
* Phiên bản tài liệu: 1.1 (Cập nhật tháng 6/2026)

## **1\. Chuẩn bị trước khi cài đặt**

Trước khi bắt đầu cấu hình, người dùng cần chuẩn bị sẵn các thông tin sau:

| Thông tin yêu cầu | Cách lấy / Ghi chú |
| :---- | :---- |
| API Key / Auth Token | Liên hệ quản trị viên để lấy  |
| Kết nối mạng | Đảm bảo kết nối Internet ổn định trong suốt quá trình thiết lập |

**CAUTION**  
Bảo mật API Key: API Key (bắt đầu bằng chuỗi ký tự fink\_pro\_...) dùng để xác thực tài khoản và trừ số dư sử dụng. Không chia sẻ mã này dưới mọi hình thức để tránh nguy cơ thất thoát tài khoản.

## **2\. Cài đặt trên Terminal và VS Code IDE**

### Yêu cầu hệ thống:

Thiết bị cần cài đặt sẵn Node.js (phiên bản 18 trở lên).

* Kiểm tra phiên bản hiện tại: Mở Terminal/PowerShell và chạy lệnh node \--version.  
* Tải và cài đặt mới: Truy cập trang chủ [nodejs.org](https://nodejs.org/) để tải phiên bản LTS mới nhất.

### Bước 1: Khởi chạy trình cài đặt tự động

1. Mở cửa sổ dòng lệnh:  
   * Windows: Sử dụng PowerShell hoặc Command Prompt (CMD).  
   * macOS / Linux: Sử dụng ứng dụng Terminal.  
2. Nhập lệnh sau và nhấn Enter:  
3. npx finkrouter

Bước 2: Trình cài đặt CLI sẽ tự động yêu cầu nhập các thông tin sau:

1. Auth Token / API Key: Dán mã API Key bắt đầu bằng fink\_pro\_... lấy từ Dashboard của Fink Router và nhấn Enter.  
2. Hệ điều hành đang sử dụng: Chọn số tương ứng và nhấn Enter:  
   * Nhập 1 cho hệ điều hành Windows  
   * Nhập 2 cho hệ điều hành macOS  
   * Nhập 3 cho hệ điều hành Linux  
3. Hệ thống sẽ hiển thị tiến trình và hoàn tất cài đặt tự động các thành phần core.

### Bước 3: Khởi chạy ứng dụng

1. Đóng cửa sổ dòng lệnh hiện tại.  
2. Mở một cửa sổ dòng lệnh mới.  
3. Chạy lệnh sau để bắt đầu phiên làm việc với Claude:  
     
4. claude

#### Video hướng dẫn:  [Claude\_Terminal.mp4](https://drive.google.com/file/d/1clMJhcPfftIYDGqtW40jN1dbKyGefzpC/view?usp=sharing)

### Bước 4: Cài đặt Extension Claude Code

1. Mở phần mềm VS Code.  
2. Sử dụng tổ hợp phím Ctrl \+ Shift \+ X (trên Windows) hoặc Cmd \+ Shift \+ X (trên macOS) để mở mục quản lý Extensions.  
3. Tìm kiếm từ khóa "Claude Code for VS Code" và nhấp chọn Install (lựa chọn extension chính thức từ Anthropic).  
4. Nhấp chọn biểu tượng Claude ở thanh công cụ phía bên trái màn hình VS Code.  
5. Chọn mô hình Opus 4.8 từ trình đơn.  
6. Nhập câu hỏi bất kỳ để kiểm tra phản hồi từ hệ thống.

#### Video hướng dẫn: [Claude\_IDE.mp4](https://drive.google.com/file/d/1B05VZ5Fw-gAyA_YxskYobqDQsSXHVqfz/view?usp=sharing)

## **3\. Cài đặt trên Claude Desktop**

### Bước 1: Cài đặt ứng dụng Claude Desktop

1. Truy cập trang tải ứng dụng của Anthropic: [claude.ai/download](https://claude.ai/download).  
2. Tải về và cài đặt phiên bản phù hợp với hệ điều hành đang sử dụng (Windows hoặc macOS).  
3. Mở ứng dụng Claude Desktop và đăng nhập bằng tài khoản Claude cá nhân.

NOTE  
Ứng dụng Claude Desktop là phần mềm được cung cấp miễn phí bởi Anthropic. Chi phí sử dụng dịch vụ chỉ được tính dựa trên lưu lượng token thực tế qua hệ thống Fink Router.

### Bước 2: Bật Developer Mode

1. Tại giao diện chính của Claude Desktop, nhấp chọn  dấu ![][image1] ở trên cùng  bên trái.  
2. Chọn **Help → Troubleshooting → Enable Developer Mode** → **Enable** 

### Bước 3: Cấu hình API Endpoint và API Key

1. Tại giao diện chính của Claude Desktop, nhấp chọn lại  dấu ![][image1] ở trên cùng  bên trái sẽ xuất hiện thêm mode **Developer**   
2. Nhấn chọn **Developer** → **Configure Third-Party Inference…** trong  giao diện   
3. Trong  giao diện **Configure third-party Inference** ở phần **Credential kind** chọn **Static API key** sau đó nhập URL là : [https://api.finkrouter.io.vn/](https://api.finkrouter.io.vn/) và  Gateway API key được cung cấp   
4.  Restart và sử dụng

#### Video hướng dẫn: [Claude\_Desktop.mp4](https://drive.google.com/file/d/186Gyq6eYawHv-YWWjLwsLaAc8zOwxSlg/view?usp=sharing)

## **4\. Xử lý sự cố thường gặp**

Dưới đây là phương án khắc phục cho các lỗi thường gặp trong quá trình cài đặt và sử dụng:

### Lỗi 1: Không xuất hiện mục "Third-party Inference" trong Claude Desktop

* Nguyên nhân: Chưa kích hoạt Developer Mode hoặc ứng dụng chưa đồng bộ thông tin tài khoản.  
* Giải pháp:  
  1. Kiểm tra lại tùy chọn Developer Mode ở cuối Settings đảm bảo đã chuyển sang trạng thái ON.  
  2. Thực hiện đăng nhập tài khoản Claude cá nhân trước khi thực hiện các bước cấu hình.  
  3. Thoát hoàn toàn ứng dụng Claude Desktop (bằng cách nhấp chuột phải vào biểu tượng ứng dụng ở khay hệ thống và chọn Quit) rồi khởi động lại.

### Lỗi 2: Hệ thống báo lỗi kết nối hoặc không nhận diện Gateway

* Nguyên nhân: Nhập sai địa chỉ Base URL hoặc API Key không hợp lệ.  
* Giải pháp:  
  1. Kiểm tra chính xác địa chỉ Base URL, đảm bảo có dấu gạch chéo / ở cuối: https://api.finkrouter.io.vn/.  
  2. Xác nhận mã API Key được dán chính xác, không dư thừa ký tự khoảng trắng ở hai đầu chuỗi.

### Lỗi 3: Lỗi thực thi khi chạy lệnh npx finkrouter

* Nguyên nhân: Node.js chưa được cài đặt hoặc phiên bản không đạt yêu cầu tối thiểu (18+).  
* Giải pháp: Tải và cài đặt lại phiên bản Node.js LTS mới nhất từ trang chủ [nodejs.org](https://nodejs.org/).  
* (Đối với macOS/Linux) Sử dụng thêm tiền tố quyền quản trị nếu bị báo lỗi phân quyền: sudo npx finkrouter.

### Lỗi 4: Lỗi lệnh "command not found" khi chạy lệnh claude

* Nguyên nhân: Biến môi trường hệ thống chưa được cập nhật sau khi cài đặt thành công.  
* Giải pháp: Đóng và mở lại một cửa sổ Terminal mới để hệ thống tải lại biến môi trường. Nếu vẫn gặp lỗi, khởi động lại máy tính hoặc chạy lại trình cài đặt tự động.

### Lỗi 5: Cuộc trò chuyện bị lỗi hoặc không nhận được phản hồi

* Nguyên nhân: Tài khoản hết số dư sử dụng (Credit) hoặc máy chủ gặp tình trạng quá tải tạm thời.  
* Giải pháp:  
  1. Đăng nhập Dashboard trên trang chủ [finkrouter.io.vn](https://finkrouter.io.vn/) để kiểm tra số dư khả dụng và thực hiện nạp thêm tiền nếu cần.  
  2. Đổi sang sử dụng mô hình AI khác (ví dụ: chuyển từ Sonnet sang Opus hoặc ngược lại) để kiểm tra tính ổn định.

## **Hỗ trợ và liên hệ**

Trường hợp gặp khó khăn kỹ thuật không thể tự khắc phục, người dùng vui lòng liên hệ bộ phận hỗ trợ qua các kênh sau:

* SDT hỗ trợ: 0778535017  
* Cộng đồng hỗ trợ: Tham gia nhóm Facebook "Non-tech học AI — Ứng dụng AI cho người bận rộn" để cùng trao đổi và nhận các tài liệu hướng dẫn bổ sung.

[image1]: <data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAEEAAAA0CAYAAADVGFU3AAAA5klEQVR4Xu3UwQ6CMBBFUX7ShA+VjxN4wTgLE58xKoXSlru4G0Knk7NoJ83L2ev8wxkDQSBEIAiECASBEIEgECIQBEIEgkCIQBAIEQjKjDAM16XvL6t6nPV5WwWCMiOUGggCIcqKwJsgEIoOBIEQZUXgTRAIRQeCQIhAEAgRCAIhAkEVIczztFvZEXyBEtodwS8ssU0RfHgtJSH4sFr7G8EHtNDPCH6wpb4i+IEW+4jgP7bcG4L/sHfTNB7eC4IvuGV+cUk9EXzp1Pyikut8+bX54JpKQvBhRzWOt6RWIfgSKflCR3QH926HfaLSLc0AAAAASUVORK5CYII=>