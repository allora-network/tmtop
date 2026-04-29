# tmtop Refactoring TODO

## Immediate Tasks


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
- **Bug Fix**: Removed signal interception that was blocking Ctrl+C, now uses tview's native input handling
- **Benefits**: No more corrupted terminal sessions after crashes or forced exits, Ctrl+C works properly

### ✅ Fix empty table display bug
- **Completed**: Implemented missing TMValidator data conversion pipeline
- **Root Issue**: Tables showed no data because validator conversion was never implemented after refactoring
- **Changes Made**:
  - Implemented `convertToTMValidators()` method in `pkg/app.go` to convert fetched JSON data to display format
  - Added `convertToCometBFTValidator()` for proper CometBFT validator creation with ed25519 key support
  - Created utility functions `HexToBytes()` and `Base64ToBytes()` in `pkg/utils/utils.go`
  - Fixed the RefreshConsensus() TODO that was blocking data flow to display layer
  - Added comprehensive error handling and logging for conversion failures
- **Benefits**: Tables now properly display validator data including names, addresses, and voting status

### ✅ Fix voting power percentage display
- **Completed**: Implemented precise voting power percentage calculation and display
- **Root Issue**: Stake percentages showed as `<nil>` because `VotingPowerPercent` field was never calculated
- **Changes Made**:
  - Enhanced `convertToTMValidators()` to calculate total voting power and individual validator percentages
  - Used `math/big.Float` for precise percentage calculations (handles fractional percentages accurately)
  - Fixed `TMValidator.Serialize()` method to properly format `*big.Float` using `.Text('f', 2)` instead of `fmt.Sprintf`
  - Added nil-checking and fallback to "0.00" for safety
  - Integrated voting power calculation with CometBFT validator creation
- **Benefits**: Stake percentages now display correctly (e.g., "15.23%") instead of `<nil>`, accurate to 2 decimal places

### ✅ Expand RoundDataMap usage to other views
- **Completed**: Extended RoundDataMap usage from `all_rounds_table.go` to `last_round_table.go` and consensus display
- **Changes Made**:
  - Updated `LastRoundTableData` to include `RoundData`, `CurrentHeight`, and `CurrentRound` fields
  - Added new methods: `SetRoundData()` and `SetCurrentRound()` for round data management
  - Created `generateValidatorDisplayText()` method that queries RoundDataMap directly instead of using manually populated `CurrentRoundVote` field
  - Updated `State` methods to use RoundDataMap queries instead of TMValidator methods for consensus display
  - Modified display wrapper to call new LastRoundTable methods with RoundDataMap and current round information
- **Technical Benefits**:
  - Consistent data source: Both LastRoundTable and AllRoundsTable now use the same RoundDataMap source
  - Simplified logic: Removed dependency on manually populated `CurrentRoundVote` field
  - Better maintainability: Centralized vote state management in RoundDataMap
  - Reduced memory usage: No need to duplicate vote state in TMValidator objects
- **Architecture Improvement**: Cleaner separation of concerns with RoundDataMap as central store for all vote data

### ✅ Implement SQLite storage for historical data
- **Completed**: Comprehensive SQLite backend with sqlc code generation for persistent consensus data storage
- **Database Schema**:
  - **Core Tables**: `validators`, `heights`, `rounds`, `votes`, `consensus_events`, `validator_snapshots`
  - **Efficient Indexing**: Optimized for height/round lookups, validator searches, and time-based queries
  - **Foreign Key Relationships**: Proper data integrity between validators, heights, rounds, and votes
- **sqlc Code Generation**:
  - **Schema**: `/pkg/db/migrations/001_initial.sql` with comprehensive table structure
  - **Queries**: Organized SQL queries in `/pkg/db/queries/` by entity type (validators, heights, rounds, votes, etc.)
  - **Generated Code**: Type-safe Go code in `/pkg/db/sqlc/` with prepared statements and interfaces
- **Service Layer Architecture**:
  - **DB Service** (`/pkg/db/db.go`): Connection management, migrations, WAL mode, cleanup routines
  - **ConsensusStore** (`/pkg/db/consensus_store.go`): High-level consensus data persistence with transaction support
  - **Integration**: Seamless integration with existing RoundDataMap and TMValidator types
- **Configuration & CLI**:
  - **New Flags**: `--database-path`, `--max-retain-blocks`, `--max-retain-days`
  - **Config File**: Full support in `config.toml.example` with comprehensive documentation
  - **Environment Variables**: `TMTOP_DATABASE_PATH`, `TMTOP_MAX_RETAIN_BLOCKS`, `TMTOP_MAX_RETAIN_DAYS`
  - **Smart Defaults**: `~/.config/tmtop/tmtop.db` if not specified
- **Real-time Persistence**:
  - **Automatic Storage**: Real-time persistence of consensus states, validator data, and vote information
  - **Event Tracking**: CometBFT events (new rounds, votes) stored with full context
  - **Background Cleanup**: Hourly cleanup routine to manage database size based on retention policies
  - **Graceful Degradation**: Application continues working if database fails to initialize
- **Data Retention Management**:
  - **Block-based Retention**: Keep last N blocks (default: 10,000)
  - **Time-based Retention**: Alternative day-based retention (default: 7 days)
  - **Automatic Cleanup**: Hourly background process removes old data while respecting foreign key constraints
- **Technical Features**:
  - **Performance**: WAL mode, prepared statements, efficient indexing, batch operations
  - **Data Integrity**: Foreign key constraints, transaction-based operations, comprehensive error handling
  - **Monitoring**: Structured logging for all database operations, cleanup status tracking
- **Benefits**: Crash resilience, historical analysis capabilities, reduced memory usage, foundation for advanced analytics