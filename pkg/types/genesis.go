package types

import "encoding/json"

type Genesis struct {
	AppState AppState `json:"app_state"`
}

type AppState struct {
	Staking json.RawMessage `json:"staking"`
	Genutil json.RawMessage `json:"genutil"`
}

type CometGenesisChunkResponse struct {
	Result *CometGenesisChunkResult `json:"result"`
}

type CometGenesisChunkResult struct {
	Chunk string `json:"chunk"`
	Total string `json:"total"`
	Data  []byte `json:"data"`
}
