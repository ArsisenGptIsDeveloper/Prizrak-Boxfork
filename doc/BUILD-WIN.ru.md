# Сборка `.exe` с нуля (Windows)

Этот проект можно собрать в Windows через Electron Forge.

## Быстрый путь (рекомендуется)

Откройте PowerShell в корне репозитория и запустите:

```powershell
powershell -ExecutionPolicy Bypass -File .\build\build-win.ps1 -Version v1.0.1 -Arch x64
```

Скрипт автоматически:

1. проверит `node`, `npm`, `go`;
2. установит npm-зависимости;
3. при необходимости установит `@electron-forge/maker-squirrel`;
4. соберёт backend `src-go\px.exe`;
5. запустит `npm run make -- --platform=win32 --arch=<arch>`;
6. покажет пути к готовым артефактам из `out\`.

## Ручной путь

```powershell
npm install
npm i -D @electron-forge/maker-squirrel@^7.8.1

cd src-go
go mod tidy
$env:CGO_ENABLED="0"
go build -tags=with_gvisor -trimpath -ldflags "-X github.com/legiz-ru/prizrak-box/api.Version=v-test" -o px.exe
cd ..

npm run make -- --platform=win32 --arch=x64
```

После этого ищите `.exe` в каталоге `out\make\...`.

## Полезные параметры скрипта

```powershell
# пропустить npm install
powershell -ExecutionPolicy Bypass -File .\build\build-win.ps1 -SkipNpmInstall

# пропустить go mod tidy
powershell -ExecutionPolicy Bypass -File .\build\build-win.ps1 -SkipGoTidy

# только собрать px.exe, без make
powershell -ExecutionPolicy Bypass -File .\build\build-win.ps1 -SkipMake
```
