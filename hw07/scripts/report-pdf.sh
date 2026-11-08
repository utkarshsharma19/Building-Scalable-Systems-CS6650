#!/usr/bin/env bash
# REPORT.md -> HW7-Utkarsh-Sharma.pdf (Markdown -> HTML -> headless Chrome).
set -euo pipefail
cd "$(dirname "$0")/.."
.venv/bin/python - <<'EOF'
import markdown, pathlib
body = markdown.markdown(pathlib.Path("REPORT.md").read_text(), extensions=["tables"])
css = """
@page { size: Letter; margin: 0.6in 0.65in; }
body { font-family: -apple-system, "Helvetica Neue", Arial, sans-serif; font-size: 10pt; line-height: 1.42; color: #1f1f1d; }
h1 { font-size: 17pt; margin: 0 0 4pt; } h2 { font-size: 13pt; margin: 16pt 0 5pt; border-bottom: 1px solid #ddd; padding-bottom: 2pt; }
h3 { font-size: 11pt; margin: 12pt 0 4pt; } h2, h3 { page-break-after: avoid; break-after: avoid; }
table { border-collapse: collapse; width: 100%; margin: 6pt 0 8pt; font-size: 8.6pt; page-break-inside: avoid; }
th, td { border: 1px solid #d6d5cf; padding: 3pt 5pt; text-align: left; vertical-align: top; } th { background: #f3f2ee; }
img { display: block; max-width: 92%; max-height: 4.1in; margin: 6pt auto 10pt; page-break-inside: avoid; }
blockquote { margin: 6pt 0; padding: 4pt 10pt; background: #f6f6f3; border-left: 3px solid #2a78d6; color: #52514e; }
code { font-size: 8.8pt; background: #f3f2ee; padding: 0 2pt; border-radius: 2px; } a { color: #2a78d6; }
"""
html = f"<!doctype html><html><head><meta charset='utf-8'><title>HW7 - Utkarsh Sharma</title><style>{css}</style></head><body>{body}</body></html>"
pathlib.Path("build").mkdir(exist_ok=True)
pathlib.Path("build/REPORT.html").write_text(html)
EOF
cp -R charts screenshots build/ 2>/dev/null || true
"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" --headless=new --disable-gpu --no-pdf-header-footer \
  --print-to-pdf="$PWD/HW7-Utkarsh-Sharma.pdf" "file://$PWD/build/REPORT.html" 2>/dev/null
ls -lh HW7-Utkarsh-Sharma.pdf
