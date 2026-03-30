# Maestro (WIP)
CLI music player for Windows

## Install

**Prerequisites:** Go 1.22+

```
go install github.com/J-Dufour/maestro@latest
```

Or build from source:

```
git clone https://github.com/J-Dufour/maestro
cd maestro
go build -o maestro.exe .
```

Move `maestro.exe` to a directory on your `PATH`.

## Usage

```
maestro <path>
```

- If `<path>` is a file, plays it directly.
- If `<path>` is a directory, plays all compatible files within it.

Multiple paths are supported:

```
maestro song.mp3 C:\Music\Albums\Jazz
```

**Supported formats:** `.mp3`, `.wav`

## Controls

| Key | Action |
|-----|--------|
| `Space` | Play / Pause |
| `k` | Skip forward |
| `j` | Skip back |
| `.` | Seek forward |
| `,` | Seek backward |
