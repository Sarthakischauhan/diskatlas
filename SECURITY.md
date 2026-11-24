# Security policy

## Reporting a vulnerability

Please report security vulnerabilities privately using [GitHub's private vulnerability reporting](https://github.com/Sarthakischauhan/diskatlas/security/advisories/new). Include the affected version or commit, steps to reproduce, impact, and any relevant logs with sensitive data removed. Please do not open a public issue for an unpatched vulnerability.

The project will review reports as time permits; there is no guaranteed response or remediation timeline. Fixes are developed against the latest `main`. There are no separately maintained security release branches.

## Safety and scope

Disk Atlas scans local files with the permissions of the user running it; it is not a sandbox. It requests confirmation before asking Finder to move a selected folder and its contents to macOS Trash. A pending tile is hidden immediately and restored if Finder reports failure. Finder permissions, external filesystem changes, and concurrent processes can affect the outcome. Review the path before confirming, and keep backups of important data. See the [user guide](docs/user-guide.md) for usage and disk-accounting limitations.
