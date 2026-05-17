# StoryForge Distribution Policy

GitHub Releases is the only trusted distribution source for StoryForge release artifacts.

## Trusted Source

Official release artifacts must be published from the StoryForge GitHub repository's Releases page.

Artifacts from package managers, mirrors, container registries, or third-party download sites are trusted only when the StoryForge project explicitly documents that they are generated from or point back to the corresponding GitHub Release.

## Release Artifacts

A complete release should include:

- platform archives, such as `storyforge_Darwin_arm64.tar.gz`, `storyforge_Linux_x86_64.tar.gz`, and `storyforge_Windows_x86_64.zip`;
- `checksums.txt`;
- release notes;
- optional package-manager manifests that reference the GitHub Release artifacts;
- optional container image metadata that references the GitHub Release tag.

Build local release artifacts with:

```bash
make release
```

## One-Command Installation

The recommended one-command installer should download only from GitHub Releases:

```bash
curl -fsSL https://raw.githubusercontent.com/smileQiny/StoryForge/main/install.sh | sh
```

The installer must:

- detect OS and CPU architecture;
- resolve the requested StoryForge version;
- download the matching archive from GitHub Releases;
- verify the archive with `checksums.txt`;
- install the `storyforge` binary into the selected install directory.

Use `STORYFORGE_VERSION` to install a specific release tag:

```bash
curl -fsSL https://raw.githubusercontent.com/smileQiny/StoryForge/main/install.sh | STORYFORGE_VERSION=v0.2.1 sh
```

## Package Managers

Package-manager support may be added for convenience, but the package metadata must reference GitHub Release artifacts as the source of truth.

Recommended order:

1. Homebrew tap for macOS and Linux.
2. GitHub Container Registry image for server deployments.
3. Scoop or WinGet for Windows.
4. `.deb` and `.rpm` packages for Linux distributions.

## Untrusted Artifacts

Do not treat a binary, archive, checksum file, package manifest, or container image as official if it cannot be traced back to a StoryForge GitHub Release.
