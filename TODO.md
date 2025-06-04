# tmtop Refactoring TODO

## Immediate Tasks

### • Expand RoundDataMap usage to other views
- **Current State**: RoundDataMap only used in `all_rounds_table.go`
- **Opportunity**: Extend to `last_round_table.go` and consensus display
- **Implementation**: Replace manual vote tracking with RoundDataMap queries
- **Benefits**: Consistent data source, simplified view logic

### • Implement SQLite storage for historical data
- **Current Issue**: All data stored in memory, limited history
- **Proposed Solution**: SQLite backend for persistence and larger datasets
- **Schema Design**: 
  - Tables for validators, votes, rounds, consensus events
  - Indexed by height/round for efficient queries
- **Benefits**: Crash resilience, historical analysis capabilities, reduced memory usage
- **Migration Strategy**: Implement alongside current in-memory storage, gradual transition


## Architectural Improvements

### • Implement centralized goroutine lifecycle management
- **Current Issue**: Multiple uncoordinated goroutines with individual timers (`app.go:71-77`)
- **Problems**: No graceful shutdown, potential race conditions, difficult debugging
- **Proposed Solution**: 
  - Single coordinator goroutine managing refresh cycles
  - Context-based cancellation for clean shutdown
  - Configurable refresh intervals per data type
- **Benefits**: Better resource management, predictable behavior, easier testing

### • Simplify State structure
- **Current Complexity**: 15+ fields in State struct with overlapping responsibilities
- **Target**: Reduce to core CometBFT types + minimal UI state
- **Example Simplified State**:
  ```go
  type State struct {
      ValidatorSet    *cometbfttypes.ValidatorSet
      ConsensusState  *cometbft.ConsensusState  
      NetworkInfo     *cometbft.NetInfo
      UIState         *DisplayState // UI-specific fields only
  }
  ```
- **Benefits**: Clearer data model, reduced memory usage, easier reasoning

### • Remove redundant type conversion layers
- **Current Issue**: Extensive conversion between custom and CometBFT types (`pkg/types/converter.go:13-145`)
- **Opportunity**: Use CometBFT types directly, leverage native JSON marshaling
- **Implementation**: Replace conversion functions with direct CometBFT type usage
- **Benefits**: Less code to maintain, better type safety, performance improvement

### • Decouple display logic from State structure
- **Current Issue**: Display layer tightly coupled to specific State structure (`pkg/display/wrapper.go:272-298`)
- **Solution**: Introduce display models or view interfaces
- **Benefits**: More flexible UI, easier testing, cleaner separation of concerns

### • Implement better error handling and observability
- **Current State**: Basic error logging, limited debugging capabilities
- **Improvements**:
  - Structured logging with context
  - Metrics collection for goroutine health
  - Better error propagation and user feedback
- **Tools**: Enhanced use of `zerolog`, potential metrics endpoint

## Research & Investigation

### • Analyze fetcher architecture consolidation opportunities
- **Question**: Can we simplify the multi-protocol support (cosmos-rpc, cosmos-lcd, tendermint)?
- **Focus**: CometBFT RPC as primary interface, reduce complexity
- **Investigation**: Review actual usage patterns of different fetcher types

### • Evaluate WebSocket vs polling trade-offs
- **Current**: Mix of WebSocket subscriptions and HTTP polling
- **Question**: Can we standardize on one approach for consistency?
- **Considerations**: Real-time requirements vs. connection reliability

### • Research CometBFT client library best practices
- **Goal**: Replace custom HTTP client with CometBFT's native RPC client
- **Benefits**: Better error handling, connection management, type safety
- **Investigation**: Review CometBFT client documentation and examples

### • Consider event-driven architecture
- **Current**: Polling-based data refresh
- **Alternative**: Event-driven updates based on blockchain events
- **Benefits**: More responsive UI, reduced resource usage
- **Challenges**: Complexity of event handling, error recovery

## Testing & Quality

### • Implement comprehensive test coverage
- **Current State**: Limited tests, mainly in `pkg/utils/`
- **Priority Areas**: State management, data conversion, goroutine coordination
- **Tools**: Standard `go test`, consider property-based testing for complex state transitions

### • Add integration tests
- **Scope**: End-to-end tests with mock blockchain data
- **Benefits**: Confidence in refactoring, regression prevention
- **Implementation**: Mock CometBFT responses, test full data flow

### • Performance profiling and optimization
- **Tools**: `go tool pprof`, memory profiling
- **Focus Areas**: RoundDataMap efficiency, goroutine resource usage
- **Baseline**: Establish current performance metrics before refactoring

## Done

### ✅ Find and implement dead code elimination tools
- **Completed**: Used official `deadcode` tool from Go team (`golang.org/x/tools/cmd/deadcode@latest`)
- **Process**: Ran `deadcode ./...` to identify unreachable functions and removed them
- **Result**: Cleaner codebase with reduced maintenance burden

### ✅ Remove Aggregator layer unnecessary abstraction
- **Completed**: Eliminated Aggregator completely from the codebase
- **Changes**: 
  - Removed `pkg/aggregator/aggregator.go` entirely
  - Moved fetcher instances directly to App struct 
  - Updated goroutine methods to call fetchers directly (`TendermintClient`, `CosmosRPCClient`, `CosmosLCDClient`)
- **Benefits**: Simplified data flow, reduced indirection, clearer responsibilities

### ✅ Consolidate redundant types in pkg/types package
- **Completed**: Unified all validator types into single `TMValidator` type
- **Removed Types**: 
  - `Validator`, `ValidatorWithRoundVote`, `ValidatorWithInfo`, `ValidatorWithChainValidator`
  - `ValidatorsWithRoundVote`, `ValidatorsWithInfo`, `ValidatorsWithInfoAndAllRoundVotes`
  - `RoundVote` (replaced with `RoundVoteState`)
- **Result**: Single `TMValidators` collection in State, significantly reduced memory usage and complexity

### ✅ Replace custom types with Cosmos SDK/CometBFT equivalents
- **Vote Types**: Replaced custom `VoteType` enum with `VoteState` aligned to CometBFT types
  - Removed: `NoVote`, `VotedNil`, `VotedForBlock` 
  - Added: `VoteStateNone`, `VoteStateNil`, `VoteStateForBlock` with CometBFT integration
- **Validator Types**: `TMValidator` now embeds `ctypes.Validator` from CometBFT as base
- **Type Integration**: Added utility functions `VoteStateFromCometBFT()`, `VoteStateFromVotesMap()`
- **Benefits**: Better type safety, reduced maintenance, leveraging battle-tested CometBFT code

### ✅ Remove all legacy/backward compatibility code
- **Completed**: Systematically removed all compatibility layers and fallback logic
- **Scope**: No external packages import tmtop, so breaking changes were safe
- **Removed**:
  - All legacy conversion functions and composite validator types
  - Backward compatibility methods in State and display layers  
  - Legacy fallback logic in display wrapper and table components
  - Obsolete `converter.go` file with transformation functions
- **Result**: Clean, unified codebase using only modern TMValidator and VoteState types

### ✅ Add config.toml support
- **Completed**: Comprehensive configuration file support using `spf13/viper`
- **Features Implemented**:
  - `--config-file` flag to specify custom config location
  - Default location: `~/.config/tmtop/config.toml` (also checks current directory)
  - All CLI flags supported in config file with kebab-case naming
  - CLI flags override config file values (proper precedence)
  - Environment variable support with `TMTOP_` prefix
  - Comprehensive example config file with documentation
- **Benefits**: Easier configuration management, reduced command-line complexity, environment-specific configs

### ✅ Fix terminal corruption on crash
- **Completed**: Implemented comprehensive terminal state restoration and cleanup system
- **Features Implemented**:
  - Signal handling (SIGINT, SIGTERM) for graceful shutdown
  - Panic recovery with terminal cleanup in all goroutines
  - Proper tcell screen cleanup with `screen.Fini()`
  - Terminal reset sequences to restore cursor and clear screen
  - Cleanup function registration system for extensibility
  - Defer-based cleanup ensures terminal is restored even on unexpected exits
- **Technical Details**:
  - Added `handleSignals()` method to catch termination signals
  - Enhanced `HandlePanic()` to perform cleanup before re-panicking
  - Implemented `restoreTerminal()` with proper tcell cleanup and ANSI reset sequences
  - Modified display wrapper to handle cleanup on drawing errors
- **Benefits**: No more corrupted terminal sessions after crashes or forced exits