package types

type CometNodeStatusResponse struct {
	Result CometNodeStatus `json:"result"`
}

type CometNodeStatus struct {
	NodeInfo      CometNodeInfo      `json:"node_info"`
	ValidatorInfo CometValidatorInfo `json:"validator_info"`
}

type CometNodeInfo struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Network string `json:"network"`
	Moniker string `json:"moniker"`
	Other   struct {
		RPCAddress string `json:"rpc_address"`
	} `json:"other"`
}
