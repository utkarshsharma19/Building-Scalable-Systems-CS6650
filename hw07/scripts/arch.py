"""Architecture diagram for the report -> charts/0-architecture.png"""
from pathlib import Path

import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt
from matplotlib.patches import FancyArrowPatch, FancyBboxPatch

OUT = Path(__file__).resolve().parent.parent / "charts" / "0-architecture.png"
INK, INK2 = "#0b0b0b", "#52514e"
BLUE, ORANGE, AQUA, VIOLET = "#2a78d6", "#eb6834", "#1baf7a", "#6250d6"

fig, ax = plt.subplots(figsize=(11, 5.2))
ax.set_xlim(0, 110)
ax.set_ylim(0, 52)
ax.axis("off")


def box(x, y, w, h, title, sub, edge, fill="#ffffff"):
    ax.add_patch(FancyBboxPatch((x, y), w, h, boxstyle="round,pad=0.4,rounding_size=1.2",
                                fc=fill, ec=edge, lw=1.6))
    ax.text(x + w / 2, y + h - 2.6, title, ha="center", va="top", fontsize=10, weight="bold", color=INK)
    ax.text(x + w / 2, y + h - 6.2, sub, ha="center", va="top", fontsize=8, color=INK2, linespacing=1.35)


def arrow(x1, y1, x2, y2, color, label="", dy=1.2, style="-"):
    ax.add_patch(FancyArrowPatch((x1, y1), (x2, y2), arrowstyle="-|>", mutation_scale=12, color=color,
                                 lw=1.6, linestyle=style))
    if label:
        ax.text((x1 + x2) / 2, (y1 + y2) / 2 + dy, label, ha="center", va="bottom", fontsize=7.5, color=INK2)


# VPC and subnets
ax.add_patch(FancyBboxPatch((15, 2), 66, 46, boxstyle="round,pad=0.3,rounding_size=1.5", fc="#f6f6f3",
                            ec="#9a998f", lw=1, linestyle="--"))
ax.text(16.5, 46.3, "VPC 10.0.0.0/16", fontsize=8, color=INK2, va="top")
ax.text(18, 42, "public 10.0.1-2.0/24", fontsize=7.5, color=INK2)
ax.text(40, 42, "private 10.0.10-11.0/24 (ECS)", fontsize=7.5, color=INK2)

box(1, 20, 12, 12, "Customers", "Locust\n20 users\n(or curl)", INK2)
box(18, 20, 14, 12, "ALB", "public\nidle timeout 60 s", BLUE)
box(39, 20, 18, 15, "Order Receiver", "ECS 256 CPU / 512 MB\n/orders/sync\n/orders/async", BLUE)
box(39, 3.5, 18, 11, "Payment (sync)", "buffered chan size 1\n3 s each → 0.33/s", ORANGE, "#fff6ef")
box(62, 22.5, 16, 10, "SNS topic", "order-processing-\nevents", VIOLET)
box(86, 33, 22, 12, "SQS queue", "order-processing-queue\nvisibility 30 s · 4 days\nlong poll 20 s", AQUA)
box(62, 3.5, 17, 13, "Order Processor", "ECS 256 CPU / 512 MB\nN worker goroutines\nN/3 orders/s", AQUA)
box(86, 6, 22, 12, "Lambda (Part III)", "provided.al2 · 512 MB\n3 s payment\ncurl tests only", VIOLET, "#f5f3fd")
box(18, 3.5, 14, 11, "NAT gateway", "private tasks →\nECR, SNS, SQS, logs", INK2)

arrow(13.8, 26, 17.6, 26, INK2)
ax.text(7, 17.5, "POST /orders/*", ha="center", fontsize=7.5, color=INK2)
arrow(32.6, 26, 38.6, 26, BLUE)
arrow(48, 19.6, 48, 15, ORANGE, "sync: wait 3 s+", dy=0.2)
arrow(57.6, 27.5, 61.6, 27.5, VIOLET)
ax.text(60, 36.5, "async: publish,\nreply 202 < 100 ms", ha="center", fontsize=7.5, color=INK2)
arrow(78.4, 30, 85.6, 37, AQUA, "raw msg")
arrow(97, 32.6, 76, 16.8, AQUA)
ax.text(99, 23.5, "ReceiveMessage\n(≤ free workers)", fontsize=7.5, color=INK2, ha="center")
arrow(78.4, 25, 85.6, 14, VIOLET, style="--")
ax.text(70, 19.2, "Part III subscription ↘", fontsize=7.5, color=VIOLET, ha="center")

ax.set_title("HW7 architecture: sync path (orange) vs. async path (SNS → SQS → workers, or SNS → Lambda)",
             fontsize=11, weight="bold", color=INK, loc="left")
fig.savefig(OUT, dpi=150, bbox_inches="tight")
print("wrote", OUT)
