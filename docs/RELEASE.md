# Guia de release — gputop

Como se publica una release de `gputop` en GitHub, y como se prepara una nueva.

## Modelo

El proyecto publica releases de forma **incremental**:

- Cada pull request abierto, sincronizado o reabierto contra `main` dispara el
  workflow `release` y publica una **prerelease**: `v<base>-pr.<n>`, construida con
  los mismos scripts que una release estable (`scripts/release.sh`,
  `scripts/build_appimage.sh`) y anclada al head del pull request.
- Cada **merge** en `main` dispara el workflow `release` y publica la **release
  estable**: `v<base>`, donde `<base>` es la version que coge de
  `cmd/gputop/main.go`.

Una release estable publica assets propios: tarball + AppImage + checksum.

## Fuente de la version

`cmd/gputop/main.go`:

```go
var version = "1.2.0"
```

La version es la fuente de verdad. `scripts/check_packaging.sh` la replica en
cuatro lugares y falla si hay desalineacion ("One version, four places"):

1. `cmd/gputop/main.go` — la fuente.
2. `packaging/man/gputop.1` — `.TH GPUTOP 1 ... "gputop <version>"`.
3. `packaging/arch/PKGBUILD` — `pkgver=<version>`.
4. `Containerfile` — `ARG VERSION=<version>`.

## Procedimiento para publicar una release (p. ej. `v1.2.0`)

1. Crear rama de release (p. ej. `release/v1.2.0`) desde `main`.
2. Bump de version en los cuatro lugares, mas las referencias de docs
   (`README.md`, `docs/DEPLOY.md`).
3. Gates: `go test ./...`, `./scripts/check_packaging.sh`, vet/lint/gofmt/build.
4. Commit (`release: bump version to 1.2.0`), pull request contra `main` y merge.
5. El push a `main` dispara el workflow `release`, que construye y publica
   `v1.2.0`.
6. Verificar: `gh release view v1.2.0`.

## Releases publicados

| Tag | Tipo | Publicada | Commit |
| --- | --- | --- | --- |
| `v1.2.0` | estable (Latest) | 2026-10-29 | `f6d8bcf` |
| `v1.2.0-pr.5` | prerelease | 2026-10-29 | PR #5 |
| `v1.1.0` | estable | 2026-10-20 | `bcc8fa7` |
| `v1.1.0-pr.1` … `v1.1.0-pr.4` | prerelease | 2026-10-20 | — |

## Comandos utiles

```bash
./scripts/podev.sh build    # binario -> ./gputop
./scripts/podev.sh test
./scripts/podev.sh vet
./scripts/podev.sh lint
./scripts/podev.sh fmt
./scripts/check_packaging.sh

./scripts/release.sh 1.2.0           # tarball + checksum -> dist/
./scripts/build_appimage.sh 1.2.0 <binary>   # -> dist/*.AppImage
```

## Notas

- `v1.0.0` es historica e inmutable: se reserva, no se republica.
- Un tag que ya existe no se mueve; si una release ya existe para la version,
  el workflow la deja intacta.
