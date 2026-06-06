#!/usr/bin/env python3
"""
Rasterize the canonical Vior icon (assets/vior-icon.svg) to PNG at any size.
Mirrors the SVG geometry exactly — both are derived from the same 1024-unit
coordinate space. Pure Pillow, no external SVG renderer required.

Usage:
    python3 rasterize_icon.py <out.png> <size>            # full icon
    python3 rasterize_icon.py <out.png> <size> foreground # transparent bg
                                                          # (for adaptive
                                                          # foreground PNGs)
"""

import sys
from PIL import Image, ImageDraw

# ── canonical palette (matches assets/vior-icon.svg) ──
BG          = (15, 17, 22, 255)        # #0f1116
MON_BODY    = (36, 42, 52, 255)        # #242a34
MON_BEZEL   = (238, 241, 245, 255)     # #eef1f5
SCREEN_DARK = (20, 23, 28, 255)        # #14171c
SCREEN_HI   = (48, 54, 64, 255)        # #303640
PHONE_BORD  = (11, 13, 16, 255)        # #0b0d10
PHONE_ORG   = (255, 138, 76, 255)      # #ff8a4c
HOME_PILL   = (255, 255, 255, 230)     # rgba(255,255,255,0.9)

# Geometry is defined in 1024-unit space; we scale to the target size.
CANVAS = 1024


def _rrect(draw, scale, x, y, w, h, r, fill=None, outline=None, stroke=0):
    """Rounded rect in 1024-unit space, scaled to pixels."""
    s = scale
    box = [round(x * s), round(y * s), round((x + w) * s), round((y + h) * s)]
    rr  = max(0, round(r * s))
    sw  = max(0, round(stroke * s))
    draw.rounded_rectangle(
        box, radius=rr, fill=fill, outline=outline, width=sw if outline else 0
    )


def _rect(draw, scale, x, y, w, h, fill):
    s = scale
    draw.rectangle(
        [round(x * s), round(y * s), round((x + w) * s), round((y + h) * s)],
        fill=fill,
    )


def render(size: int, transparent_bg: bool = False) -> Image.Image:
    """Render the icon at the requested square pixel size.

    Uses 4x supersampling for clean edges, then downsamples to `size`.
    """
    ss = 4
    big = size * ss
    scale = big / CANVAS

    img = Image.new("RGBA", (big, big), (0, 0, 0, 0))
    d   = ImageDraw.Draw(img)

    # Background squircle — skip for adaptive-icon foreground variants.
    if not transparent_bg:
        _rrect(d, scale, 0, 0, 1024, 1024, 225, fill=BG)

    # Monitor body: dark fill + light bezel stroke
    _rrect(d, scale, 170, 270, 550, 350, 40,
           fill=MON_BODY, outline=MON_BEZEL, stroke=22)

    # Inner screen (darker)
    _rrect(d, scale, 200, 300, 490, 290, 16, fill=SCREEN_DARK)

    # Top highlight bar inside the screen
    _rrect(d, scale, 216, 316, 458, 40, 6, fill=SCREEN_HI)

    # Stand neck (square)
    _rect(d, scale, 420, 620, 84, 100, fill=MON_BEZEL)

    # Stand base bar
    _rrect(d, scale, 270, 730, 384, 30, 12, fill=MON_BEZEL)

    # Phone dark separation border
    _rrect(d, scale, 540, 420, 290, 460, 46, fill=PHONE_BORD)

    # Phone body (orange)
    _rrect(d, scale, 560, 440, 250, 420, 34, fill=PHONE_ORG)

    # Phone inner screen
    _rrect(d, scale, 580, 510, 210, 310, 10, fill=SCREEN_DARK)

    # Phone home indicator pill
    _rrect(d, scale, 635, 830, 100, 14, 7, fill=HOME_PILL)

    return img.resize((size, size), Image.LANCZOS)


def main():
    if len(sys.argv) < 3:
        sys.exit("usage: rasterize_icon.py <out.png> <size> [foreground]")
    out_path = sys.argv[1]
    size     = int(sys.argv[2])
    fg_only  = len(sys.argv) > 3 and sys.argv[3] == "foreground"
    img      = render(size, transparent_bg=fg_only)
    img.save(out_path, "PNG", optimize=True)
    print(f"wrote {out_path} ({size}x{size}{' foreground' if fg_only else ''})")


if __name__ == "__main__":
    main()
