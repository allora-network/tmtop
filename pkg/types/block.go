package types

import "time"

type CometBlockResponse struct {
	Result CometBlockResult `json:"result"`
}

type CometBlockResult struct {
	Block *CometBlock `json:"block"`
}

type CometBlock struct {
	Header CometBlockHeader `json:"header"`
}

type CometBlockHeader struct {
	Height string    `json:"height"`
	Time   time.Time `json:"time"`
}
