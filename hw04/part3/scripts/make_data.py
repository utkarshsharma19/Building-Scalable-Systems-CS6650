import sys
from pathlib import Path

SIZES_MB = [1, 5, 10, 25, 50]

src = Path(sys.argv[1]).read_text(encoding="utf-8", errors="ignore")
out_dir = Path(__file__).parent.parent / "data"
out_dir.mkdir(exist_ok=True)

for mb in SIZES_MB:
    target = mb * 1024 * 1024
    text = (src + "\n") * (target // len(src.encode()) + 1)
    data = text.encode()[:target]
    data = data[: data.rfind(b" ")]
    (out_dir / f"text_{mb}MB.txt").write_bytes(data)
    print(f"text_{mb}MB.txt  {len(data):,} bytes")
