$ErrorActionPreference = "Stop"
$RepoRoot = Split-Path -Parent $PSScriptRoot
$OllamaDir = Join-Path $RepoRoot "tools\ollama"
$OllamaExe = Join-Path $OllamaDir "ollama.exe"
$ModelsDir = Join-Path $RepoRoot "storage\models"
$ZipPath = Join-Path $OllamaDir "ollama-windows-amd64.zip"
$DownloadUrl = "https://github.com/ollama/ollama/releases/download/v0.40.18/ollama-windows-amd64.zip"

New-Item -ItemType Directory -Force -Path $OllamaDir | Out-Null
New-Item -ItemType Directory -Force -Path $ModelsDir | Out-Null

if (-not (Test-Path $OllamaExe)) {
    Write-Host "Downloading portable Ollama into $OllamaDir (supports resume)..."
    & curl.exe -fL -C - --retry 5 -o $ZipPath $DownloadUrl
    if ($LASTEXITCODE -ne 0) {
        throw "Failed to download Ollama archive (exit code $LASTEXITCODE)"
    }
    Write-Host "Extracting Ollama..."
    & tar -xf $ZipPath -C $OllamaDir
    if ($LASTEXITCODE -ne 0) {
        throw "Failed to extract Ollama archive"
    }
    Remove-Item $ZipPath -Force
}

$env:OLLAMA_MODELS = $ModelsDir
$env:OLLAMA_HOST = "127.0.0.1:11434"
$env:OLLAMA_MAX_LOADED_MODELS = "1"
$env:OLLAMA_KEEP_ALIVE = "2m"
$env:OLLAMA_NUM_PARALLEL = "1"

Write-Host "Starting temporary local Ollama server to pull qwen2.5:3b into $ModelsDir..."
$proc = Start-Process -FilePath $OllamaExe -ArgumentList "serve" -PassThru -WindowStyle Hidden
try {
    Start-Sleep -Seconds 3
    & $OllamaExe pull qwen2.5:3b
    if ($LASTEXITCODE -ne 0) {
        throw "Failed to pull qwen2.5:3b"
    }
    Write-Host "Done! Ollama & qwen2.5:3b are installed inside $RepoRoot"
}
finally {
    if ($proc -and -not $proc.HasExited) {
        Stop-Process -Id $proc.Id -Force
    }
}
