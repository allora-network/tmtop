package main

import (
	"main/pkg"
	configPkg "main/pkg/config"
	"main/pkg/logger"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	version = "unknown"
)

func Execute(inputConfig configPkg.InputConfig, args []string, configFile string) {
	// Load configuration from file if specified
	if err := loadConfigFile(configFile, &inputConfig); err != nil {
		logger.GetDefaultLogger().Fatal().Err(err).Msg("Could not load configuration file")
	}

	// Override RPC host with positional argument if provided
	if len(args) > 0 && args[0] != "" {
		inputConfig.RPCHost = args[0]
	} else if inputConfig.RPCHost == "" {
		inputConfig.RPCHost = "http://localhost:26657"
	}

	config, err := configPkg.ParseAndValidateConfig(inputConfig)
	if err != nil {
		panic(err)
	}

	app := pkg.NewApp(config, version)
	app.Start()
}

// loadConfigFile loads configuration from file using viper
func loadConfigFile(configFile string, config *configPkg.InputConfig) error {
	v := viper.New()

	// Set config file path
	if configFile != "" {
		// Use specified config file
		v.SetConfigFile(configFile)
	} else {
		// Use default locations
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		
		configDir := filepath.Join(homeDir, ".config", "tmtop")
		v.SetConfigName("config")
		v.SetConfigType("toml")
		v.AddConfigPath(configDir)      // ~/.config/tmtop/
		v.AddConfigPath(".")            // current directory
	}

	// Set environment variable prefix for automatic env binding
	v.SetEnvPrefix("TMTOP")
	v.AutomaticEnv()

	// Map config keys to viper keys (using kebab-case for consistency with CLI flags)
	v.BindEnv("rpc-host", "TMTOP_RPC_HOST")
	v.BindEnv("provider-rpc-host", "TMTOP_PROVIDER_RPC_HOST")
	v.BindEnv("consumer-id", "TMTOP_CONSUMER_ID")
	v.BindEnv("refresh-rate", "TMTOP_REFRESH_RATE")
	v.BindEnv("validators-refresh-rate", "TMTOP_VALIDATORS_REFRESH_RATE")
	v.BindEnv("chain-info-refresh-rate", "TMTOP_CHAIN_INFO_REFRESH_RATE")
	v.BindEnv("upgrade-refresh-rate", "TMTOP_UPGRADE_REFRESH_RATE")
	v.BindEnv("block-time-refresh-rate", "TMTOP_BLOCK_TIME_REFRESH_RATE")
	v.BindEnv("chain-type", "TMTOP_CHAIN_TYPE")
	v.BindEnv("verbose", "TMTOP_VERBOSE")
	v.BindEnv("disable-emojis", "TMTOP_DISABLE_EMOJIS")
	v.BindEnv("debug-file", "TMTOP_DEBUG_FILE")
	v.BindEnv("halt-height", "TMTOP_HALT_HEIGHT")
	v.BindEnv("blocks-behind", "TMTOP_BLOCKS_BEHIND")
	v.BindEnv("lcd-host", "TMTOP_LCD_HOST")
	v.BindEnv("timezone", "TMTOP_TIMEZONE")
	v.BindEnv("with-topology-api", "TMTOP_WITH_TOPOLOGY_API")
	v.BindEnv("topology-listen-addr", "TMTOP_TOPOLOGY_LISTEN_ADDR")

	// Read config file
	if err := v.ReadInConfig(); err != nil {
		// Config file not found is acceptable - CLI flags/defaults will be used
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return err
		}
	}

	// Map viper values to config struct (only if they exist and are not zero values)
	if v.IsSet("rpc-host") {
		config.RPCHost = v.GetString("rpc-host")
	}
	if v.IsSet("provider-rpc-host") {
		config.ProviderRPCHost = v.GetString("provider-rpc-host")
	}
	if v.IsSet("consumer-id") {
		config.ConsumerID = v.GetString("consumer-id")
	}
	if v.IsSet("refresh-rate") {
		config.RefreshRate = v.GetDuration("refresh-rate")
	}
	if v.IsSet("validators-refresh-rate") {
		config.ValidatorsRefreshRate = v.GetDuration("validators-refresh-rate")
	}
	if v.IsSet("chain-info-refresh-rate") {
		config.ChainInfoRefreshRate = v.GetDuration("chain-info-refresh-rate")
	}
	if v.IsSet("upgrade-refresh-rate") {
		config.UpgradeRefreshRate = v.GetDuration("upgrade-refresh-rate")
	}
	if v.IsSet("block-time-refresh-rate") {
		config.BlockTimeRefreshRate = v.GetDuration("block-time-refresh-rate")
	}
	if v.IsSet("chain-type") {
		config.ChainType = v.GetString("chain-type")
	}
	if v.IsSet("verbose") {
		config.Verbose = v.GetBool("verbose")
	}
	if v.IsSet("disable-emojis") {
		config.DisableEmojis = v.GetBool("disable-emojis")
	}
	if v.IsSet("debug-file") {
		config.DebugFile = v.GetString("debug-file")
	}
	if v.IsSet("halt-height") {
		config.HaltHeight = v.GetInt64("halt-height")
	}
	if v.IsSet("blocks-behind") {
		config.BlocksBehind = v.GetUint64("blocks-behind")
	}
	if v.IsSet("lcd-host") {
		config.LCDHost = v.GetString("lcd-host")
	}
	if v.IsSet("timezone") {
		config.Timezone = v.GetString("timezone")
	}
	if v.IsSet("with-topology-api") {
		config.WithTopologyAPI = v.GetBool("with-topology-api")
	}
	if v.IsSet("topology-listen-addr") {
		config.TopologyListenAddr = v.GetString("topology-listen-addr")
	}

	return nil
}

func main() {
	var config configPkg.InputConfig
	var configFile string

	rootCmd := &cobra.Command{
		Use:     "tmtop [RPC host URL]",
		Long:    "Observe the pre-voting status of any Tendermint-based blockchain.",
		Version: version,
		Args:    cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			Execute(config, args, configFile)
		},
	}

	// Configuration file flag
	rootCmd.PersistentFlags().StringVar(&configFile, "config-file", "", "Path to configuration file (default: ~/.config/tmtop/config.toml)")

	// All existing flags
	rootCmd.PersistentFlags().StringVar(&config.ProviderRPCHost, "provider-rpc-host", "", "Provider chain RPC host URL")
	rootCmd.PersistentFlags().StringVar(&config.ConsumerID, "consumer-id", "", "Consumer ID (not chain ID!)")
	rootCmd.PersistentFlags().DurationVar(&config.RefreshRate, "refresh-rate", time.Second, "Refresh rate")
	rootCmd.PersistentFlags().BoolVar(&config.Verbose, "verbose", false, "Display more debug logs")
	rootCmd.PersistentFlags().BoolVar(&config.DisableEmojis, "disable-emojis", false, "Disable emojis in output")
	rootCmd.PersistentFlags().StringVar(&config.ChainType, "chain-type", "cosmos-rpc", "Chain type. Allowed values are: 'cosmos-rpc', 'cosmos-lcd', 'tendermint'")
	rootCmd.PersistentFlags().DurationVar(&config.ValidatorsRefreshRate, "validators-refresh-rate", time.Minute, "Validators refresh rate")
	rootCmd.PersistentFlags().DurationVar(&config.ChainInfoRefreshRate, "chain-info-refresh-rate", 5*time.Minute, "Chain info refresh rate")
	rootCmd.PersistentFlags().DurationVar(&config.UpgradeRefreshRate, "upgrade-refresh-rate", 30*time.Minute, "Upgrades refresh rate")
	rootCmd.PersistentFlags().DurationVar(&config.BlockTimeRefreshRate, "block-time-refresh-rate", 30*time.Second, "Block time refresh rate")
	rootCmd.PersistentFlags().StringVar(&config.LCDHost, "lcd-host", "", "LCD API host URL")
	rootCmd.PersistentFlags().StringVar(&config.DebugFile, "debug-file", "", "Path to file to write debug info/logs to")
	rootCmd.PersistentFlags().Int64Var(&config.HaltHeight, "halt-height", 0, "Custom halt-height")
	rootCmd.PersistentFlags().Uint64Var(&config.BlocksBehind, "blocks-behind", 1000, "How many blocks behind to check to calculate block time")
	rootCmd.PersistentFlags().StringVar(&config.Timezone, "timezone", "", "Timezone to display dates in")
	rootCmd.PersistentFlags().BoolVar(&config.WithTopologyAPI, "with-topology-api", false, "Enable topology API")
	rootCmd.PersistentFlags().StringVar(&config.TopologyListenAddr, "topology-listen-addr", "0.0.0.0:8080", "The address on which to serve topology API")

	if err := rootCmd.Execute(); err != nil {
		logger.GetDefaultLogger().Fatal().Err(err).Msg("Could not start application")
	}
}
