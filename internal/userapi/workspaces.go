package userapi

import "context"

// Workspace is one workspace the authenticated user belongs to, flattened from
// the `me { memberships { role workspace {...} } }` shape so the CLI can print
// one row per workspace with the caller's role attached.
type Workspace struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	Role        string `json:"role"`
}

// ListWorkspaces returns the workspaces the authenticated user belongs to. It
// reuses the `me` query (the same path the web app's workspace switcher uses),
// so the request is user-scoped and carries no X-Workspace header.
func (c *Client) ListWorkspaces(ctx context.Context) ([]Workspace, error) {
	var resp struct {
		Me *struct {
			Memberships []struct {
				Role      string `json:"role"`
				Workspace struct {
					Name        string `json:"name"`
					DisplayName string `json:"displayName"`
					Description string `json:"description"`
				} `json:"workspace"`
			} `json:"memberships"`
		} `json:"me"`
	}
	err := c.Do(ctx, Request{
		Query: "query ListWorkspaces { me { memberships { role workspace { name displayName description } } } }",
	}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Me == nil {
		return nil, errMissingData("me")
	}

	workspaces := make([]Workspace, 0, len(resp.Me.Memberships))
	for _, m := range resp.Me.Memberships {
		workspaces = append(workspaces, Workspace{
			Name:        m.Workspace.Name,
			DisplayName: m.Workspace.DisplayName,
			Description: m.Workspace.Description,
			Role:        m.Role,
		})
	}
	return workspaces, nil
}
