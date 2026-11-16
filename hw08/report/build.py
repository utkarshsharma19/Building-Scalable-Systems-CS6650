#!/usr/bin/env python3
"""Build the consolidated team report: the original 26-page team PDF with Utkarsh's sections inserted.

Markdown sections in report/ -> HTML with the original report's styling -> headless Chrome PDFs
-> merged with the original pages by PyMuPDF. Figures are {figure:key|path|caption} (one) or
{figures:k1|p1|c1|k2|p2|c2} (two side by side), referenced in text as {fig:key}; they're numbered
in page order starting after the original's last figure (22).

usage: .venv/bin/python report/build.py ORIGINAL.pdf OUTPUT.pdf
"""
import pathlib
import re
import subprocess
import sys

import markdown
import pymupdf

HERE = pathlib.Path(__file__).resolve().parent
ROOT = HERE.parent
BUILD = HERE / "build"
CHROME = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
FIRST_FIG = 23

# Matches the original (headless Chrome, Letter, Helvetica Neue 9.8pt, 44pt margins).
CSS = """
@page { size: Letter; margin: 0.57in 0.6in 0.6in 0.6in; }
* { box-sizing: border-box; }
body { font-family: "Helvetica Neue", Helvetica, Arial, sans-serif; font-size: 9.8pt; line-height: 1.5; color: #111; margin: 0; }
h1 { font-size: 16pt; margin: 0 0 14pt; padding-bottom: 5pt; border-bottom: 1px solid #222; }
h1.title { font-size: 20.9pt; border: 0; margin: 0 0 4pt; padding: 0; }
h2 { font-size: 13pt; background: #f1f1f1; border-left: 3px solid #333; padding: 6pt 9pt; margin: 18pt 0 12pt; }
h2:first-child { margin-top: 0; }
h3 { font-size: 11.8pt; margin: 15pt 0 6pt; }
h4 { font-size: 10.2pt; margin: 12pt 0 5pt; }
h2, h3, h4 { page-break-after: avoid; break-after: avoid; }
p { margin: 0 0 7pt; }
ul { margin: 0 0 7pt; padding-left: 15pt; } li { margin: 0 0 3pt; }
table { border-collapse: collapse; width: 100%; margin: 4pt 0 9pt; font-size: 8.6pt; line-height: 1.35; page-break-inside: avoid; break-inside: avoid; }
th, td { border: 1px solid #ddd; padding: 4pt 5pt; text-align: left; vertical-align: top; }
th { background: #f3f3f3; font-weight: bold; }
code { font-family: Menlo, monospace; font-size: 8.4pt; }
pre { background: #f6f6f6; border: 1px solid #e5e5e5; padding: 6pt 8pt; font-size: 7.9pt; line-height: 1.4; white-space: pre-wrap; margin: 4pt 0 9pt; page-break-inside: avoid; }
pre code { font-size: 7.9pt; }
figure { margin: 6pt 0 10pt; page-break-inside: avoid; break-inside: avoid; }
figure img { display: block; max-width: 100%; max-height: 3.3in; margin: 0 auto; border: 1px solid #e5e5e5; }
figcaption { font-size: 8.6pt; margin-top: 3pt; color: #222; }
.pair { display: flex; gap: 14pt; margin: 6pt 0 10pt; page-break-inside: avoid; break-inside: avoid; }
.pair figure { flex: 1; margin: 0; } .pair img { max-height: 2.4in; }
.mono { font-family: Menlo, monospace; }
"""

# Document order of the new sections (figure numbering follows it).
SECTIONS = ["page1", "step1-utkarsh", "step1-team", "step2-utkarsh", "step2-team", "step3-utkarsh", "step3-team"]


def number_figures(texts):
    nums, n = {}, FIRST_FIG
    for t in texts:
        for m in re.finditer(r"\{figures?:([^}]*)\}", t):
            parts = m.group(1).split("|")
            for k in parts[0::3]:
                nums[k] = n
                n += 1
    return nums


def fig_html(key, path, cap, nums):
    src = (ROOT / path).resolve().as_uri()
    return f'<figure><img src="{src}"><figcaption>Figure {nums[key]}) {cap}</figcaption></figure>'


def render(name, text, nums):
    def one(m):
        k, p, c = m.group(1).split("|", 2)
        return "\n\n" + fig_html(k, p, c, nums) + "\n\n"

    def two(m):
        a = m.group(1).split("|")
        return '\n\n<div class="pair">' + fig_html(*a[0:3], nums) + fig_html(*a[3:6], nums) + "</div>\n\n"

    text = re.sub(r"\{figure:([^}]*)\}", one, text)
    text = re.sub(r"\{figures:([^}]*)\}", two, text)
    text = re.sub(r"\{fig:([\w-]+)\}", lambda m: f"Figure {nums[m.group(1)]}", text)
    body = markdown.markdown(text, extensions=["tables", "fenced_code", "md_in_html"])
    html = f"<!doctype html><html><head><meta charset='utf-8'><title>{name}</title><style>{CSS}</style></head><body>{body}</body></html>"
    BUILD.mkdir(exist_ok=True)
    hp, pp = BUILD / f"{name}.html", BUILD / f"{name}.pdf"
    hp.write_text(html)
    subprocess.run([CHROME, "--headless=new", "--disable-gpu", "--no-pdf-header-footer", "--allow-file-access-from-files",
                    f"--print-to-pdf={pp}", hp.as_uri()], check=True, capture_output=True)
    return pymupdf.open(pp)


def main():
    orig_path, out_path = sys.argv[1], sys.argv[2]
    texts = {s: (HERE / f"{s}.md").read_text() for s in SECTIONS}
    nums = number_figures([texts[s] for s in SECTIONS])
    parts = {s: render(s, texts[s], nums) for s in SECTIONS}
    # STEP II: my section and the 3-way team comparison flow together
    orig = pymupdf.open(orig_path)

    # Page 16: keep Shikha's implementation notes, drop the old 2-way Team comparison below them.
    p16 = orig[15]
    hit = p16.search_for("Team comparison")[0]
    p16.add_redact_annot(pymupdf.Rect(0, hit.y0 - 14, p16.rect.width, p16.rect.height), fill=(1, 1, 1))
    p16.apply_redactions(images=pymupdf.PDF_REDACT_IMAGE_REMOVE)

    out = pymupdf.open()

    def orig_pages(a, b):  # 1-based inclusive
        out.insert_pdf(orig, from_page=a - 1, to_page=b - 1)

    out.insert_pdf(parts["page1"])          # new page 1 (Utkarsh added to the team and the table)
    orig_pages(2, 9)                        # STEP I: Eason, Tian, Shikha
    out.insert_pdf(parts["step1-utkarsh"])  # STEP I: Utkarsh
    orig_pages(10, 11)                      # STEP I team comparison (original)
    out.insert_pdf(parts["step1-team"])     # STEP I team comparison: Utkarsh's MySQL added
    orig_pages(12, 16)                      # STEP II: Eason, Shikha (p16 trimmed)
    out.insert_pdf(parts["step2-utkarsh"])  # STEP II: Utkarsh
    out.insert_pdf(parts["step2-team"])     # STEP II team comparison, now 3-way (replaces p16 bottom + p17)
    orig_pages(18, 22)                      # STEP III: Eason, Shikha
    out.insert_pdf(parts["step3-utkarsh"])  # STEP III: Utkarsh
    orig_pages(23, 26)                      # STEP III team comparison + Team Summary (original)
    out.insert_pdf(parts["step3-team"])     # STEP III team comparison: Utkarsh added, updated summary
    out.set_metadata({**orig.metadata, "title": "CS6650 HW8 Team Report"})
    out.save(out_path, garbage=3, deflate=True)
    print(f"{out_path}: {out.page_count} pages (original {orig.page_count}); figures {FIRST_FIG}-{max(nums.values())}")
    for s in SECTIONS:
        print(f"  {s}: {parts[s].page_count} page(s)")


if __name__ == "__main__":
    main()
