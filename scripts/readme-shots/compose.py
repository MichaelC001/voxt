#!/usr/bin/env python3
"""Puts the README's Mac captures (mac-shots.sh) on the wallpaper and
writes them as WebP into docs/images.

  python3 scripts/readme-shots/compose.py --wallpaper WALL CAPTURES OUT [--png DIR]

WALL is the desktop picture the user chose (not in the repository: only
the composed pictures are). Every picture is as wide as WALL; only its
height is cropped (around the middle), never its width, nor stretched.
CAPTURES holds en/ and zh-Hans/ (build/readme-shots, pulled from the
Mac): each window as screencapture -l took it, with macOS's shadow on a
transparent ground, at 2x. OUT gets <shot>.webp (English) and
<shot>-zh.webp (Chinese). Window shots (home, history, dictionary, models)
are the window alone on the wallpaper. Overlay shots (transcribe,
translate, rewrite) are composed: the editor window, also a real capture,
with the overlay's capture below it, where VoxtMini shows it (at the
bottom of the screen), both at the same scale; the desktop around them is
the wallpaper, not a real screen. --png also writes PNGs there.
"""
import argparse
import os

from PIL import Image

WINDOW_SHOTS = ["home", "history", "dictionary", "models"]
OVERLAY_SHOTS = ["transcribe", "translate", "rewrite"]
LANGS = {"en": "", "zh-Hans": "-zh"}
MARGIN = 0.05  # of the width, around the overlay shots
WINDOW_W = 0.87  # the window shots' window, of the width
PAD = 0.04  # above and below it, of the width


def crop_height(wall, h):
    """WALL's full width, h of its height around its middle."""
    h = min(h, wall.height)
    y = (wall.height - h) // 2
    return wall.crop((0, y, wall.width, y + h)).convert("RGBA")


def scaled(img, s):
    return img.resize((round(img.width * s), round(img.height * s)), Image.LANCZOS)


def body(img):
    """The window itself in a capture: the box of its opaque pixels
    (the shadow around it is translucent)."""
    return img.getchannel("A").point(lambda a: 255 if a > 250 else 0).getbbox()


def window_shot(wall, cap):
    """The window in the middle of the wallpaper, WINDOW_W of its width,
    with PAD of the width above and below (the shadow shows there)."""
    W = wall.width
    l, t, r, b = body(cap)
    s = min(W * WINDOW_W / (r - l), (wall.height - 2 * W * PAD) / (b - t))
    win = scaled(cap, s)
    h = round((b - t) * s + 2 * W * PAD)
    canvas = crop_height(wall, h)
    # The window's body in the middle; its shadow may run off the edges.
    x = round(W / 2 - (l + r) / 2 * s)
    y = round(canvas.height / 2 - (t + b) / 2 * s)
    paste_clipped(canvas, win, x, y)
    return canvas


def paste_clipped(canvas, img, x, y):
    """alpha_composite, img possibly running off canvas."""
    cx, cy = max(0, -x), max(0, -y)
    part = img.crop((cx, cy, min(img.width, canvas.width - x), min(img.height, canvas.height - y)))
    canvas.alpha_composite(part, (max(0, x), max(0, y)))


def overlay_shot(wall, editor, overlay):
    """The editor window, and the overlay where VoxtMini shows it: below,
    in the middle, over the window's lower edge; one scale for both."""
    W, m = wall.width, round(wall.width * MARGIN)
    s = min((W * 0.80) / editor.width, (W - 2 * m) / overlay.width)
    ed, ov = scaled(editor, s), scaled(overlay, s)
    overlap = round(ed.height * 0.22)
    h = m + ed.height - overlap + ov.height + m // 2
    if h > wall.height:  # too tall: the same, smaller
        f = (wall.height - 1.5 * m) / (h - 1.5 * m)
        ed, ov, overlap = scaled(ed, f), scaled(ov, f), round(overlap * f)
        h = wall.height
    canvas = crop_height(wall, h)
    canvas.alpha_composite(ed, ((W - ed.width) // 2, m))
    canvas.alpha_composite(ov, ((W - ov.width) // 2, m + ed.height - overlap))
    return canvas


def save(img, out, name, png_dir):
    rgb = img.convert("RGB")
    rgb.save(os.path.join(out, name + ".webp"), "WEBP", quality=84, method=6)
    if png_dir:
        rgb.save(os.path.join(png_dir, name + ".png"))


def main():
    p = argparse.ArgumentParser()
    p.add_argument("--wallpaper", required=True)
    p.add_argument("captures")
    p.add_argument("out")
    p.add_argument("--png")
    a = p.parse_args()
    wall = Image.open(a.wallpaper).convert("RGB")
    os.makedirs(a.out, exist_ok=True)
    if a.png:
        os.makedirs(a.png, exist_ok=True)
    for lang, suffix in LANGS.items():
        d = os.path.join(a.captures, lang)
        have = lambda n: os.path.exists(os.path.join(d, n + ".png"))  # noqa: E731
        load = lambda n: Image.open(os.path.join(d, n + ".png")).convert("RGBA")  # noqa: E731
        for n in WINDOW_SHOTS:
            if have(n):
                save(window_shot(wall, load(n)), a.out, n + suffix, a.png)
        if not have("editor"):
            continue
        editor = load("editor")
        for n in OVERLAY_SHOTS:
            if have(n):
                save(overlay_shot(wall, editor, load(n)), a.out, n + suffix, a.png)


if __name__ == "__main__":
    main()
