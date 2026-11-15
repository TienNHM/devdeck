# Cài đặt và chạy devdeck

[English](INSTALL.md)

devdeck chỉ là một file chạy. Không có trình cài đặt, và không có gì chạy ngầm trừ khi bạn tự mở nó.

## 1. Tải về

### Windows

**Cách A: tải file**
1. Mở [bản phát hành mới nhất](https://github.com/TienNHM/devdeck/releases/latest) và tải `devdeck_<phiên bản>_windows_amd64.zip` (máy ARM thì chọn `windows_arm64`).
2. Giải nén vào một thư mục cố định, ví dụ `C:\Tools\devdeck\`.
3. Bấm đúp `devdeck.exe`, hoặc chạy trong terminal:
   ```powershell
   C:\Tools\devdeck\devdeck.exe --open
   ```
   File exe chưa được ký số, nên Windows SmartScreen có thể báo "Windows protected your PC". Bấm **More info → Run anyway**.

**Cách B: Scoop**
```powershell
scoop install https://raw.githubusercontent.com/TienNHM/devdeck/main/scoop/devdeck.json
devdeck --open
```

### macOS
```bash
# Chip Apple: darwin_arm64, chip Intel: darwin_amd64
curl -LO https://github.com/TienNHM/devdeck/releases/download/v0.2.0/devdeck_v0.2.0_darwin_arm64.tar.gz
tar -xzf devdeck_v0.2.0_darwin_arm64.tar.gz
cd devdeck_v0.2.0_darwin_arm64
xattr -d com.apple.quarantine ./devdeck 2>/dev/null   # file chưa được Apple notarize
./devdeck --open
```

### Linux
```bash
# x86-64: linux_amd64, ARM: linux_arm64
curl -LO https://github.com/TienNHM/devdeck/releases/download/v0.2.0/devdeck_v0.2.0_linux_amd64.tar.gz
tar -xzf devdeck_v0.2.0_linux_amd64.tar.gz
sudo install devdeck_v0.2.0_linux_amd64/devdeck /usr/local/bin/devdeck   # không bắt buộc
devdeck --open
```

### Build từ mã nguồn
Cần Go 1.27+ và git.
```bash
git clone https://github.com/TienNHM/devdeck
cd devdeck
go build -trimpath -ldflags "-s -w" -o devdeck.exe .   # trên macOS/Linux đặt tên là "devdeck"
./devdeck.exe --open
```

## 2. Lần chạy đầu tiên

Khi khởi động, devdeck in ra dạng:
```
devdeck v0.2.0 listening on 127.0.0.1 only
open: http://127.0.0.1:7077/?t=3f9c…
```
- Có `--open` thì trình duyệt tự mở link đó; nếu không, hãy copy link vào trình duyệt.
- Phần `?t=…` là token truy cập. Chỉ cần ở lần đầu; sau đó trình duyệt nhớ bạn, nên mở `http://127.0.0.1:7077/` là đủ.
- Đừng đóng cửa sổ terminal: đóng nó là devdeck dừng.

Nên thử trước:
- **Ctrl+K** mở bảng lệnh (thử `kill port 3000`, `ssh`, `go git`).
- **Dự án → Thêm dự án**, dán đường dẫn thư mục, rồi chạy một task được tìm thấy.
- **Git**: thêm một repo rồi chuột phải vào commit hoặc nhánh.
- Nút quả địa cầu đổi English / Tiếng Việt; nút mặt trăng đổi sáng / tối.

## 3. Tuỳ chọn

| Lệnh | Tác dụng |
|---|---|
| `devdeck` | Chạy dashboard ở port 7077 |
| `devdeck --port 7080` | Dùng port khác (khi 7077 đang bận) |
| `devdeck --open` | Mở luôn trình duyệt |
| `devdeck version` | In phiên bản |
| `devdeck hooks-config` | In hướng dẫn kết nối Claude Code (xem bên dưới) |
| `devdeck mcp` | MCP server cho Claude Code (Claude tự chạy lệnh này) |

## 4. Kết nối Claude Code (tuỳ chọn)

1. Chạy `devdeck hooks-config`. Lệnh này in ra hai phần.
2. Chạy dòng `claude mcp add devdeck -- "<đường dẫn>" mcp` vừa in ra. Từ giờ Claude Code dùng được các tool của devdeck (port, git, Redis, database, Docker, log, SSH…).
3. Gộp khối `"hooks"` vừa in vào `~/.claude/settings.json` (trên Windows: `C:\Users\<tên bạn>\.claude\settings.json`). Mở một phiên Claude Code mới; phiên đó sẽ hiện trong tab **Claude**.
4. Trong devdeck, bấm biểu tượng chuông để nhận thông báo trên màn hình khi Claude cần bạn hoặc làm xong.

Nút **Claude → Cài đặt** hiện cùng nội dung và cho biết hook đã cài đủ chưa. devdeck không bao giờ sửa file cài đặt của Claude.

Nếu sau này bạn chuyển `devdeck.exe` sang chỗ khác, chạy `claude mcp remove devdeck` rồi làm lại bước 2–3 với đường dẫn mới.

## 5. Dừng, chạy cùng máy, cập nhật, gỡ bỏ

**Dừng:** bấm **Thoát** (góc trên bên phải), hoặc đóng cửa sổ terminal / nhấn Ctrl+C.

**Tự chạy khi đăng nhập**
- Windows: nhấn Win+R, gõ `shell:startup`, rồi tạo shortcut tới `devdeck.exe` trong thư mục đó. Trong Properties của shortcut, chọn **Run: Minimized**.
- Linux (systemd user service):
  ```bash
  mkdir -p ~/.config/systemd/user
  cat > ~/.config/systemd/user/devdeck.service <<'EOF'
  [Unit]
  Description=devdeck
  [Service]
  ExecStart=/usr/local/bin/devdeck
  Restart=on-failure
  [Install]
  WantedBy=default.target
  EOF
  systemctl --user enable --now devdeck
  ```
- macOS: thêm một launchd agent, hoặc chạy `devdeck` trong file profile của shell.

**Cập nhật:** dừng devdeck, thay file chạy bằng file của bản mới, rồi chạy lại. Cài đặt của bạn vẫn được giữ.

**Gỡ bỏ:** xoá file chạy và thư mục `~/.devdeck` (`C:\Users\<tên bạn>\.devdeck` trên Windows). Nếu đã kết nối Claude Code, chạy `claude mcp remove devdeck` và xoá các hook của devdeck trong `~/.claude/settings.json`.

## 6. Dữ liệu nằm ở đâu

Mọi thứ nằm trong `~/.devdeck/`: token truy cập, host đã lưu, dự án, repo, profile Redis/database, tunnel và danh sách file log. Mật khẩu không bao giờ được lưu ở đó.

## 7. Xử lý sự cố

| Vấn đề | Cách xử lý |
|---|---|
| `cannot listen: … bind: …` (Windows: "Only one usage of each socket address", macOS/Linux: "address already in use") | devdeck (hoặc chương trình khác) đang dùng port 7077. Mở `http://127.0.0.1:7077/`, hoặc chạy với `--port 7080` |
| Trình duyệt báo `unauthorized` | Mở link đầy đủ có `?t=…` một lần. Mất link? Token nằm trong `~/.devdeck/token`, hoặc chạy lại devdeck để in lại |
| Trang báo `forbidden host` | Dùng `127.0.0.1` hoặc `localhost`. devdeck cố ý từ chối các tên máy khác |
| Tab Docker báo Docker chưa chạy | Mở Docker Desktop (hoặc docker daemon) rồi bấm Thử lại |
| Tiến trình hiện `(không có quyền truy cập)` | Tiến trình đó thuộc hệ thống hoặc người dùng khác. Chỉ chạy devdeck bằng quyền admin nếu thật sự cần xem |
| Tab Claude trống | Kiểm tra **Claude → Cài đặt**; hook phải có trong settings.json và devdeck phải đang chạy khi Claude khởi động |
| Terminal bị hẹp hoặc trống | Đổi kích thước cửa sổ một lần; nếu vẫn bị, tải lại trang (terminal vẫn giữ sau khi reload) |
