# Windows Support (WSL)

Kandev runs on Windows through WSL (Windows Subsystem for Linux).

## Prerequisites

- Windows 10 (version 2004+) or Windows 11
- WSL 2 with a Linux distribution installed (Ubuntu recommended)

## Setup

### 1. Install WSL

From PowerShell (as Administrator):

```powershell
wsl --install
```

Restart your machine if prompted, then open your Linux distribution from the Start menu.

### 2. Install system dependencies

```bash
sudo apt update
sudo apt install build-essential libatomic1 git
```

- `build-essential` provides `gcc`, required by `go-sqlite3` (CGO)
- `libatomic1` is required by Node.js

### 3. Install mise (runtime manager)

```bash
curl https://mise.run | sh
```

Add mise to your shell profile:

```bash
echo 'eval "$(~/.local/bin/mise activate bash)"' >> ~/.bashrc
source ~/.bashrc
```

### 4. Install runtimes

```bash
mise use -g node
mise use -g pnpm
mise use -g go
```

### 5. Clone and install

```bash
git clone <repo-url> kandev
cd kandev
make install
```

### 6. Run

```bash
make dev    # development mode
make start  # production mode
```

## Troubleshooting

### `libatomic.so.1: cannot open shared object file`

Node.js requires `libatomic1`. Install it:

```bash
sudo apt install libatomic1
```

### `pnpm: command not found`

Ensure pnpm is installed and activated via mise:

```bash
mise use -g pnpm
```

### `Binary was compiled with 'CGO_ENABLED=0', go-sqlite3 requires cgo`

The SQLite driver needs a C compiler. Install `build-essential`:

```bash
sudo apt install build-essential
```

### Browser doesn't auto-open

WSL interop must be enabled so the CLI can launch your Windows browser. Ensure `/etc/wsl.conf` contains:

```ini
[interop]
enabled=true
appendWindowsPath=true
```

Then restart WSL from PowerShell:

```powershell
wsl --shutdown
```

### `cd: can't cd to apps` during `make install`

This is a symptom of pnpm not being installed. Follow step 4 above.
