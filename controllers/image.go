package controllers

import (
	"strings"

	"github.com/casosorg/casos/store"
)

// GetImageConfig returns a container image's own runtime metadata as the
// registry holds it: exposed ports, declared volumes, shipped env and OCI
// labels. The install dialog prefills itself from this so an image can be
// deployed without anyone reading its documentation first.
// @router /api/get-image-config [get]
func (c *ApiController) GetImageConfig() {
	if c.RequireSignedIn() {
		return
	}
	image := strings.TrimSpace(c.GetString("image"))
	if image == "" {
		c.ResponseError("image parameter is required")
		return
	}
	config, err := store.GetImageConfig(c.Ctx.Request.Context(), image, c.GetString("platform"))
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(config)
}
