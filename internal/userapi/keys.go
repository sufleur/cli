package userapi

import "context"

// RevokeUserAPIKey revokes the key with the given id. Returns true if the
// backend confirms the key was revoked; false if it reports no rows changed.
func (c *Client) RevokeUserAPIKey(ctx context.Context, id string) (bool, error) {
	var resp struct {
		RevokeUserAPIKey bool `json:"revokeUserApiKey"`
	}
	err := c.Do(ctx, Request{
		Query:     "mutation RevokeUserApiKey($id: ID!) { revokeUserApiKey(id: $id) }",
		Variables: map[string]any{"id": id},
	}, &resp)
	if err != nil {
		return false, err
	}
	return resp.RevokeUserAPIKey, nil
}
