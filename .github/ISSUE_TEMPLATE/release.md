---
name: Release
about: Track a new release of ping_exporter
title: 'Release v[VERSION]'
labels: 'release'
assignees: ''

---

## Release Checklist

### Pre-release
- [ ] All planned features and fixes are merged to main
- [ ] All tests are passing
- [ ] Documentation is updated
- [ ] IMPLEMENTATION.md reflects current state
- [ ] Version number decided: `v[VERSION]`

### Release Process
- [ ] Create and push Git tag:
  ```bash
  git tag -a v[VERSION] -m "Release version [VERSION]"
  git push origin v[VERSION]
  ```
- [ ] Verify GitHub Actions workflows complete successfully
- [ ] Verify Docker images are published to quay.io/zebbra/ping_exporter
- [ ] Verify GitHub release is created with binaries
- [ ] Test Docker image: `docker run --rm quay.io/zebbra/ping_exporter:v[VERSION] --version`

### Post-release
- [ ] Update any deployment configurations
- [ ] Announce release (if applicable)
- [ ] Close this issue

### Release Notes

Brief description of changes in this release:

- 
- 
- 

### Breaking Changes

List any breaking changes (if any):

- 
- 

### Links

- [ ] GitHub Release: https://github.com/zebbra/ping_exporter/releases/tag/v[VERSION]
- [ ] Docker Image: https://quay.io/repository/zebbra/ping_exporter?tab=tags&tag=v[VERSION]