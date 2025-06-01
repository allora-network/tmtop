# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and Development Commands

```bash
# Build the complete application (frontend + Go binary)
make build

# Build just the Go binary
make build-go

# Build just the frontend
make build-front

# Install the binary to $GOPATH/bin
make install

# Run linting with automatic fixes
make lint

# Run tests with coverage
make test
```

## Project Architecture

tmtop is a terminal-based monitoring tool for Tendermint/CometBFT consensus visualization. The application follows a layered architecture:

### Core Components

- **pkg/app.go**: Main application orchestrator that coordinates all goroutines and manages state
- **pkg/aggregator/**: Data collection layer that fetches consensus state, validators, and chain info from multiple sources
- **pkg/fetcher/**: Protocol-specific data fetchers (Cosmos RPC, LCD, Tendermint, WebSocket)
- **pkg/display/**: Terminal UI layer using tview for consensus visualization 
- **pkg/types/**: Shared data structures and state management
- **pkg/topology/**: Network topology API and embedded frontend

### Key Patterns

- **Concurrent Data Fetching**: Multiple goroutines refresh different data types at configurable intervals
- **State Management**: Centralized state in `types.State` with thread-safe access patterns
- **Mailbox Pattern**: Uses `github.com/brynbellomy/go-utils` mailboxes for async message passing
- **Protocol Abstraction**: `DataFetcher` interface allows supporting different chain types (cosmos-rpc, cosmos-lcd, tendermint)

### Data Flow

1. App starts multiple refresh goroutines (consensus, validators, chain info, etc.)
2. Aggregator coordinates data fetching from configured endpoints
3. State is updated and propagated to display layer
4. Terminal UI renders tables showing consensus progress, validator status, and chain info

### Frontend Integration

The project includes a React/TypeScript frontend in `pkg/topology/embed/frontend/` that provides a web-based topology view. Frontend assets are embedded into the Go binary.

## Configuration

All configuration is handled via CLI flags in `cmd/tmtop.go`. Key parameters:
- RPC host (required)
- Provider RPC host (for consumer chains) 
- Chain type (cosmos-rpc, cosmos-lcd, tendermint)
- Various refresh rates for different data types
- Topology API settings

## Testing

Tests are located alongside source files with `_test.go` suffix. Current test coverage focuses on utility functions in `pkg/utils/`.