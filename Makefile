.PHONY: build dev dev-frontend frontend test clean

# Build the full production binary (frontend + Go).
build: frontend
	go build -o dist/deploykitd ./cmd/deploykitd

# Build only the frontend.
frontend:
	cd frontend && npm install && npm run build
	rm -rf http/spa_assets/dist
	cp -r frontend/build/client http/spa_assets/dist

# Run Go backend for development (no embedded SPA).
dev:
	go run -tags dev ./cmd/deploykitd

# Run frontend dev server (in a separate terminal).
dev-frontend:
	cd frontend && npm run dev

# Run all tests.
test:
	go test ./...

# Clean build artifacts.
clean:
	rm -rf dist/ http/spa_assets/dist/ frontend/node_modules/
