# Viewer

Run from the repo root:

```powershell
cd viewer
npm start
```

Then open:

```text
http://127.0.0.1:4173/viewer/
```

The page loads JSON exports from the repo, including:

```text
/replays/Match-2026-02-15_18-28-03-10012-R01.transforms.json
```

Current scope:
- Local "Control Room" panel for browsing replay rounds and folders under `replays/`.
- One-click movement export, probe export, and Blender camera export from the browser.
- Batch conversion for replay folders directly from the viewer.
- Follow-camera style validation with a chosen actor locked to screen center.
- Plane switching for `XY`, `XZ`, and `YZ` views.
- Sample-order scrubbing for merged position/rotation samples.
- No actor-to-username mapping yet.
- No reliable replay-clock timestamps yet, so the viewer treats the track as untimed.

Browser note:
- The server is fully local.
- This first version loads React from https://esm.sh, so the page needs internet access in the browser.
