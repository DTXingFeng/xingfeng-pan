"""Verify the built upload UI with native XHR and an isolated local API.

Run `npm run build` first, then `python tests/upload_browser_test.py`.
Requires Python Playwright and its Chromium browser.
"""

import argparse
import json
import re
import tempfile
import threading
import time
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import parse_qs, urlparse

from playwright.sync_api import expect, sync_playwright


DIST = Path(__file__).resolve().parents[1] / "dist"


class TestAPI(SimpleHTTPRequestHandler):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, directory=str(DIST), **kwargs)

    def log_message(self, *args):
        pass

    def respond(self, data, status=200, message="ok"):
        body = json.dumps({"code": status, "data": data, "message": message}).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        url = urlparse(self.path)
        if url.path == "/api/auth/check":
            self.respond({"authenticated": True})
        elif url.path == "/api/list":
            self.server.list_requests += 1
            path = parse_qs(url.query).get("path", [""])[0]
            self.respond({"path": path, "parent": "", "files": [], "page": 1, "total": 0})
        elif url.path == "/api/stats":
            self.respond({"total_files": 0, "total_size": 0})
        elif url.path.startswith("/api/"):
            self.respond({})
        else:
            if url.path == "/files":
                self.path = "/index.html"
            super().do_GET()

    def do_POST(self):
        remaining = int(self.headers.get("Content-Length", "0"))
        slow_upload = remaining > 1024 * 1024
        parts = []
        while remaining:
            chunk = self.rfile.read(min(64 * 1024, remaining))
            if not chunk:
                break
            parts.append(chunk)
            remaining -= len(chunk)
            if slow_upload:
                time.sleep(0.03)
        body = b"".join(parts)
        if urlparse(self.path).path == "/api/mkdir":
            self.respond({})
            return
        filename = re.search(rb'filename="([^"]+)"', body).group(1).decode().replace('\\', '/').rsplit('/', 1)[-1]
        # Keep the server-processing stage observable after the bytes arrive.
        time.sleep(1.5)
        if filename == "failed.txt":
            self.respond({}, 500, "测试磁盘空间不足")
        elif filename == "duplicate.txt":
            self.respond([{"name": filename, "skipped": True}])
        elif filename == "renamed.txt":
            self.respond([{"name": "renamed_1.txt", "renamed": True}])
        else:
            self.respond([{"name": filename}])


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--screenshots", type=Path)
    args = parser.parse_args()
    if args.screenshots:
        args.screenshots.mkdir(parents=True, exist_ok=True)

    server = ThreadingHTTPServer(("127.0.0.1", 0), TestAPI)
    server.list_requests = 0
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        with sync_playwright() as playwright:
            browser = playwright.chromium.launch(headless=True)
            page = browser.new_page(viewport={"width": 1440, "height": 900})
            errors = []
            warnings = []
            page.on("pageerror", lambda error: errors.append(str(error)))
            page.on("console", lambda message: warnings.append(message.text) if message.type == "warning" else None)
            page.goto(f"http://127.0.0.1:{server.server_port}/files")
            page.wait_for_load_state("networkidle")
            page.wait_for_function("navigator.serviceWorker.controller !== null")
            baseline_warnings = {warning.splitlines()[0] for warning in warnings}
            assert page.locator('input[type="file"]').count() == 2
            upload_input = page.locator('input[type="file"]:not([webkitdirectory])')
            folder_input = page.locator('input[webkitdirectory]')

            def screenshot(name):
                if args.screenshots:
                    page.screenshot(path=str(args.screenshots / f"{name}.png"), animations="disabled")

            page.evaluate("""() => {
                window.__uploadEvents = [];
                const send = XMLHttpRequest.prototype.send;
                XMLHttpRequest.prototype.send = function(...args) {
                    this.upload.addEventListener('progress', event => {
                        window.__uploadEvents.push({ loaded: event.loaded, total: event.total });
                    });
                    return send.apply(this, args);
                };
            }""")
            upload_input.set_input_files({"name": "progress.bin", "mimeType": "application/octet-stream", "buffer": b"x" * (16 * 1024 * 1024)})
            toast = page.locator('.upload-toast')
            progress = toast.locator('[role="progressbar"]').first
            try:
                page.wait_for_function("""() => {
                    const progress = document.querySelector('.upload-toast [role="progressbar"]');
                    const value = Number(progress?.getAttribute('aria-valuenow'));
                    return value > 0 && value < 99;
                }""")
            except Exception:
                screenshot("upload-debug")
                print(toast.inner_html(), flush=True)
                print(page.evaluate("window.__uploadEvents"), flush=True)
                print(errors, flush=True)
                raise
            expect(toast.get_by_text('上传中', exact=True)).to_be_visible()
            expect(toast.locator('.upload-speed')).to_be_visible()
            screenshot("upload-progress")
            expect(toast.get_by_text('服务器处理中', exact=True)).to_be_visible(timeout=20000)
            expect(progress).to_have_attribute('aria-valuenow', '99')
            screenshot("upload-processing")
            expect(toast.get_by_text('上传成功', exact=True)).to_be_visible()
            expect(progress).to_have_attribute('aria-valuenow', '100')
            assert server.list_requests >= 2, 'Upload completion must fetch a fresh file list'
            native_progress = page.evaluate("window.__uploadEvents")
            assert any(0 < event['loaded'] < event['total'] for event in native_progress)
            page.wait_for_timeout(1800)
            expect(toast).to_be_visible()
            screenshot("upload-complete")
            print("PASS: native XHR shows intermediate progress, processing, success and persistent completion")

            toast.get_by_role('button', name='关闭', exact=True).click()
            expect(toast).not_to_be_visible()
            with tempfile.TemporaryDirectory(prefix="upload-ui-test-") as directory:
                folder = Path(directory) / "folder"
                folder.mkdir()
                for name in ["duplicate.txt", "failed.txt", "renamed.txt"]:
                    (folder / name).write_text("test contents", encoding="utf8")
                folder_input.set_input_files(str(folder))
                expect(toast.locator('.upload-toast-item')).to_have_count(3)
                expect(toast.get_by_text('等待上传', exact=True)).to_be_visible()
                expect(toast.get_by_role('button', name='清除已完成')).to_be_disabled()
                expect(toast.get_by_text('重复跳过', exact=True)).to_be_visible()
                expect(toast.get_by_text('上传失败', exact=True)).to_be_visible()
                expect(toast.get_by_text('保存为：renamed_1.txt', exact=True)).to_be_visible()
                expect(toast.get_by_text('测试磁盘空间不足', exact=True)).to_be_visible()
                expect(toast.locator('.upload-summary')).to_have_text('已结束 · 1/3 个失败')
                assert server.list_requests >= 3, 'Folder completion must fetch a fresh file list'
                page.wait_for_timeout(1800)
                expect(toast.locator('.upload-toast-item')).to_have_count(3)
            screenshot("upload-mixed-results")
            print("PASS: folder queue and skipped/failed/renamed results remain visible")

            page.set_viewport_size({"width": 390, "height": 844})
            page.evaluate("document.documentElement.classList.add('dark')")
            expect(page.locator('.el-message')).to_have_count(0)
            expect(toast).to_be_visible()
            bounds = toast.bounding_box()
            assert bounds['x'] >= 0 and bounds['x'] + bounds['width'] <= 390
            assert bounds['y'] + bounds['height'] <= 844
            screenshot("upload-mobile-dark")
            toast.get_by_role('button', name='关闭', exact=True).click()
            expect(toast).not_to_be_visible()
            assert not errors, errors
            new_warnings = [warning for warning in warnings if warning.splitlines()[0] not in baseline_warnings]
            assert not new_warnings, new_warnings
            print("PASS: manual cleanup, mobile/dark layout and no new browser errors or warnings")
            browser.close()
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=2)


if __name__ == "__main__":
    main()
