# The README's screenshots

The pictures in `docs/images/` that README.md (English) and README.zh-CN.md
(Chinese) show: real captures of VoxtMini's windows on a Mac, put on a
desktop picture the user chose.

| File | What it shows | Real or composed |
|---|---|---|
| `history`, `models`, `dictionary` (`-zh`) | the main window | the real window, captured with macOS's shadow, on the wallpaper |
| `transcribe`, `translate`, `rewrite` (`-zh`) | the overlay: live text, a translation's card, a Rewrite conversation | the real overlay and a real editor window (VoxtMini's own, with made-up notes), arranged on the wallpaper as a desktop would show them |

The history, the dictionary and every text are made up
(`app/readme_shots_data.go`); the models are files of the right sizes with
no bytes in a temporary library.

## Regenerating

1. On the Mac (it builds VoxtMini with `-tags readmeshots` as a bundle of
   its own, `dev.hehehai.voxtmini.readmeshots`, over a temporary library:
   never the installed VoxtMini or its data; it comes to the front for
   about half a minute only when nobody used the Mac for 5 minutes, then
   gives the front back):

   ```sh
   MAC_DIR=code/VoxtMini-shots scripts/mac.sh scripts/readme-shots/mac-shots.sh
   MAC_DIR=code/VoxtMini-shots scripts/mac.sh --pull build/readme-shots
   ```

   `VOXTMINI_SHOTS_DRY=1` keeps it in the background (inactive windows);
   `VOXTMINI_SHOTS_BIN=<binary>` runs a binary built elsewhere
   (`CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -tags readmeshots ./app`)
   instead of building on the Mac.

2. Here: `python3 scripts/readme-shots/compose.py --wallpaper WALL.jpg build/readme-shots docs/images`
   (Home is captured too, not used.)

The pages with the same data render on Linux too, for a look before a Mac
run: `VOXTMINI_README_SHOTS=/tmp/readme go test -tags readmeshots -run TestReadmeShotsPages ./app`.

## The wallpaper

The user's own desktop picture (1600 px wide, dark navy and violet
waves), kept outside the repository: only the composed pictures are in
it. Every picture is as wide as it; compose.py crops only its height.
