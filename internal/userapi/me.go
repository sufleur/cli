package userapi

import "context"

// Me is the subset of the user record the CLI currently surfaces.
type Me struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

// Me runs the `me` query and returns the authenticated user.
func (c *Client) Me(ctx context.Context) (*Me, error) {
	var resp struct {
		Me *Me `json:"me"`
	}
	if err := c.Do(ctx, Request{Query: "query Me { me { id email } }"}, &resp); err != nil {
		return nil, err
	}
	if resp.Me == nil {
		return nil, errMissingData("me")
	}
	return resp.Me, nil
}
