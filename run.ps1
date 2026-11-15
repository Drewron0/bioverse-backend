param(
    [Parameter(Position=0)]
    [string]$Command = "help"
)

switch ($Command.ToLower()) {
    "test" {
        Write-Host "Running tests..." -ForegroundColor Cyan
        go test -v -count=1 ./...
    }
    "lint" {
        Write-Host "Running go vet..." -ForegroundColor Cyan
        go vet ./...
    }
    "openapi" {
        Write-Host "Linting OpenAPI spec..." -ForegroundColor Cyan
        npx --yes @redocly/cli lint api/openapi.yaml
    }
    "api" {
        Write-Host "Starting API server..." -ForegroundColor Green
        go run cmd/api/main.go
    }
    "worker" {
        Write-Host "Starting background worker..." -ForegroundColor Green
        go run cmd/worker/main.go
    }
    "sqlc" {
        Write-Host "Running sqlc generate..." -ForegroundColor Cyan
        sqlc generate
    }
    default {
        Write-Host "BioVerse Windows Helper Tasks:" -ForegroundColor Yellow
        Write-Host "  .\run.ps1 test     - Run all tests"
        Write-Host "  .\run.ps1 lint     - Run go vet"
        Write-Host "  .\run.ps1 openapi  - Lint OpenAPI specification"
        Write-Host "  .\run.ps1 api      - Run API server"
        Write-Host "  .\run.ps1 worker   - Run background worker"
        Write-Host "  .\run.ps1 sqlc     - Generate sqlc code"
    }
}
