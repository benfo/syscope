# Installs syscope on Windows from the latest GitHub release.
#
#   irm https://raw.githubusercontent.com/benfo/syscope/master/install.ps1 | iex
#
# Environment:
#   SYSCOPE_VERSION      release tag to install (default: the newest, e.g. v0.1.0)
#   SYSCOPE_INSTALL_DIR  where to put syscope.exe (default: %LOCALAPPDATA%\Programs\syscope)
#
# Run it again to upgrade. To uninstall, delete the folder and remove it from
# your user Path.

# A function, so errors stop the install without closing the caller's shell.
function Install-Syscope {
	$ErrorActionPreference = 'Stop'
	$ProgressPreference = 'SilentlyContinue' # Windows PowerShell downloads crawl with the progress bar
	[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

	$repo = 'benfo/syscope'
	$installDir = $env:SYSCOPE_INSTALL_DIR
	if (-not $installDir) { $installDir = Join-Path $env:LOCALAPPDATA 'Programs\syscope' }

	$cpu = $env:PROCESSOR_ARCHITEW6432 # set when 32-bit PowerShell runs on 64-bit Windows
	if (-not $cpu) { $cpu = $env:PROCESSOR_ARCHITECTURE }
	switch ($cpu) {
		'AMD64' { $arch = 'amd64' }
		'ARM64' { $arch = 'arm64' }
		default { throw "syscope install: unsupported CPU $cpu" }
	}

	# The newest full release, or the newest pre-release while there are only
	# pre-releases (GitHub's "latest" skips those).
	$version = $env:SYSCOPE_VERSION
	if (-not $version) {
		try {
			$version = (Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest").tag_name
		} catch {
			$version = @(Invoke-RestMethod "https://api.github.com/repos/$repo/releases?per_page=1")[0].tag_name
		}
	}
	if (-not $version) { throw "syscope install: couldn't find a release of $repo" }

	$archive = "syscope_$($version.TrimStart('v'))_windows_$arch.zip"
	$base = "https://github.com/$repo/releases/download/$version"
	$tmp = Join-Path ([IO.Path]::GetTempPath()) "syscope-install-$([guid]::NewGuid())"
	New-Item -ItemType Directory $tmp | Out-Null
	try {
		Write-Host "Downloading syscope $version for windows/$arch..."
		$zip = Join-Path $tmp $archive
		Invoke-WebRequest "$base/$archive" -OutFile $zip -UseBasicParsing
		$sums = (Invoke-WebRequest "$base/SHA256SUMS" -UseBasicParsing).Content
		if ($sums -is [byte[]]) { $sums = [Text.Encoding]::UTF8.GetString($sums) }

		# Lines are "<hash>  <file>", or "<hash> *<file>" in binary mode.
		$want = ($sums -split "`n" | Where-Object { $_ -match " \*?$([regex]::Escape($archive))\s*$" }) -replace ' .*', ''
		$got = (Get-FileHash $zip -Algorithm SHA256).Hash
		if (-not $want -or $want -ne $got) { throw "syscope install: checksum mismatch for $archive" }

		Expand-Archive $zip -DestinationPath $tmp
		New-Item -ItemType Directory -Force $installDir | Out-Null
		$exe = Join-Path $installDir 'syscope.exe'
		# A running exe can't be overwritten but can be renamed, so upgrading
		# works while syscope is open.
		$old = "$exe.old"
		Remove-Item $old -Force -ErrorAction SilentlyContinue
		if (Test-Path $exe) { Move-Item $exe $old -Force }
		Move-Item (Join-Path $tmp 'syscope.exe') $exe
		Remove-Item $old -Force -ErrorAction SilentlyContinue
	} finally {
		Remove-Item $tmp -Recurse -Force -ErrorAction SilentlyContinue
	}

	$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
	$onPath = ($userPath -split ';') -contains $installDir
	if (-not $onPath) {
		[Environment]::SetEnvironmentVariable('Path', (@($userPath, $installDir) | Where-Object { $_ }) -join ';', 'User')
	}
	if (($env:Path -split ';') -notcontains $installDir) { $env:Path += ";$installDir" }

	Write-Host "Installed $(& $exe --version) to $exe"
	if (-not $onPath) { Write-Host "Added $installDir to your user Path (other open terminals need a restart)." }
	Write-Host 'Run: syscope'
}

Install-Syscope
