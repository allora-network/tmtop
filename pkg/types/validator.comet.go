package types

type CometValidatorInfo struct {
	Address     string `json:"address"`
	VotingPower string `json:"voting_power"`
}

type CometValidatorsResponse struct {
	Result *CometValidatorsResult `json:"result"`
	Error  *CometValidatorsError  `json:"error"`
}

type CometValidatorsError struct {
	Message string `json:"message"`
	Data    string `json:"data"`
}

type CometValidatorsResult struct {
	Count      string           `json:"count"`
	Total      string           `json:"total"`
	Validators []CometValidator `json:"validators"`
}

type CometValidator struct {
	Address     string               `json:"address"`
	VotingPower string               `json:"voting_power"`
	PubKey      CometValidatorPubKey `json:"pub_key"`
}

type CometValidatorPubKey struct {
	Type         string `json:"type"`
	PubKeyBase64 string `json:"value"`
}
