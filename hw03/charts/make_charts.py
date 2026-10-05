from pathlib import Path

OUT = Path(__file__).parent
COLORS = ["#4C78A8", "#F58518", "#54A24B", "#B279A2"]


def grouped_bars(path, title, unit, groups, series, values, fmt="{:.1f}"):
    W, H = 720, 380
    left, right, top, bottom = 70, 20, 60, 70
    pw, ph = W - left - right, H - top - bottom
    raw = max(max(row) for row in values) * 1.15 / 5
    mag = 10 ** (len(str(int(raw))) - 1) if raw >= 1 else 1
    step = next(m * mag for m in (1, 2, 5, 10) if m * mag >= raw)
    vmax = step * 5
    gw = pw / len(groups)
    bw = gw * 0.7 / len(series)

    s = [f'<svg xmlns="http://www.w3.org/2000/svg" width="{W}" height="{H}" '
         f'font-family="Helvetica, Arial, sans-serif" font-size="12">',
         f'<rect width="{W}" height="{H}" fill="#ffffff"/>',
         f'<text x="{W/2}" y="24" text-anchor="middle" font-size="16" font-weight="bold" fill="#222">{title}</text>']

    for i in range(6):
        v = vmax * i / 5
        y = top + ph - ph * i / 5
        s.append(f'<line x1="{left}" y1="{y:.1f}" x2="{W-right}" y2="{y:.1f}" stroke="#e5e5e5"/>')
        s.append(f'<text x="{left-8}" y="{y+4:.1f}" text-anchor="end" fill="#555">{v:,.0f}</text>')
    s.append(f'<text x="18" y="{top+ph/2}" transform="rotate(-90 18 {top+ph/2})" '
             f'text-anchor="middle" fill="#555">{unit}</text>')

    for g, name in enumerate(groups):
        gx = left + g * gw + gw * 0.15
        for k, _ in enumerate(series):
            v = values[g][k]
            h = ph * v / vmax
            x, y = gx + k * bw, top + ph - h
            s.append(f'<rect x="{x:.1f}" y="{y:.1f}" width="{bw-3:.1f}" height="{h:.1f}" '
                     f'fill="{COLORS[k]}" rx="2"/>')
            s.append(f'<text x="{x+(bw-3)/2:.1f}" y="{y-5:.1f}" text-anchor="middle" '
                     f'font-size="11" fill="#222">{fmt.format(v)}</text>')
        s.append(f'<text x="{left+g*gw+gw/2:.1f}" y="{top+ph+20}" text-anchor="middle" '
                 f'fill="#222">{name}</text>')

    lx = left
    for k, name in enumerate(series):
        s.append(f'<rect x="{lx}" y="{H-24}" width="12" height="12" fill="{COLORS[k]}"/>')
        s.append(f'<text x="{lx+18}" y="{H-14}" fill="#222">{name}</text>')
        lx += 30 + 7 * len(name)
    s.append("</svg>")
    path.write_text("\n".join(s))


grouped_bars(OUT / "maps.svg", "50 goroutines x 1,000 ops: time to finish (lower is better)", "ms",
             ["Write-only", "90% reads"], ["Mutex", "RWMutex", "sync.Map"],
             [[8.47, 8.53, 3.91], [5.90, 3.31, 2.09]], fmt="{:.2f}")

grouped_bars(OUT / "locust_rps.svg", "Locust throughput, 50 users, GET:POST = 3:1 (higher is better)",
             "requests / second", ["1 worker", "4 workers"], ["HttpUser", "FastHttpUser"],
             [[4238, 13118], [11788, 41138]], fmt="{:,.0f}")
print("wrote", OUT / "maps.svg", OUT / "locust_rps.svg")
