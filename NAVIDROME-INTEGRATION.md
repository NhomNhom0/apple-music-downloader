# Navidrome download integration decisions

Recorded: 2026-09-16. Deployment has not been performed.

## Accepted audio settings

- Store ALAC in M4A; no automatic FLAC conversion.
- Set `alac-max: 48000` and `convert-after-download: false`.
- Keep `storefront: "vn"`.
- Preserve the selected source bit depth: 16-bit/44.1 kHz is suitable;
  24-bit/44.1 kHz and 24-bit/48 kHz are also acceptable.
- The sample-rate cap is not a bit-depth cap. Do not upsample, force
  16-bit output, or silently use a higher-rate source on failure.
- Validate actual codec, sample rate, bit depth and full audio decoding
  before importing. Report unavailable or incompatible sources.
- Existing downloads are unchanged; the setting applies to future runs.
- Local backend remains `http://127.0.0.1:12340`.
- Feishin with MPV is the preferred playback test; verify direct playback,
  seeking and track changes on the actual listening device.

## Proposed manager pipeline

URL -> metadata -> duplicate check -> persistent job queue -> download worker
-> staging -> audio and tag validation -> atomic import -> Navidrome scan.

- Start with one download job at a time.
- Persist jobs and authentication state across restarts.
- Keep partial files outside the scanned library; perform the final rename
  on the same filesystem. Cross-filesystem copies require a destination
  staging step before rename.
- Keep wrapper endpoints private; manager exposes authenticated job controls.
- Do not put credentials, tokens or signed URLs in logs or this document.
- Use process supervision, health checks and bounded log retention.
- Avoid adding download/conversion work to the existing sleep-based
  opencode-host container without first fixing service lifecycle management.

## Deployment decision still open

Option A: dedicated Kubernetes workload(s) for worker and wrapper, with
persistent volumes and appropriate runtime support. Prefer this if the
cluster can support the wrapper runtime and shared storage cleanly.

Option B: supervised containers/services on the AudioMuse host
(currently reached at 192.168.1.254:6000), with a private worker API and
shared music storage. This is a practical pilot if CPU/RAM headroom,
architecture, runtime support and storage permissions are verified first.

Do not assume the AudioMuse host is less loaded: its resources have not
been audited. Audio analysis and lossless conversion can compete for CPU.
Do not migrate the Mac QEMU bundle unchanged; select the target OS/CPU build.
The manager should call the same job API regardless of worker placement.
