package controllers

import (
	"encoding/json"

	"github.com/casosorg/casos/object"
)

func (c *ApiController) GetGlobalSites() {
	if c.RequireAdmin() {
		return
	}

	sites, err := object.GetGlobalSites()
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	c.ResponseOk(sites)
}

func (c *ApiController) GetSites() {
	if c.RequireAdmin() {
		return
	}

	sites, err := object.GetGlobalSites()
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	c.ResponseOk(sites)
}

func (c *ApiController) GetSite() {
	id := c.Input().Get("id")

	site, err := object.GetSite(id)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	c.ResponseOk(site)
}

func (c *ApiController) GetBuiltInSite() {
	site, err := object.GetBuiltInSite()
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	if site == nil {
		c.ResponseOk(nil)
		return
	}
	c.ResponseOk(struct {
		Owner         string `json:"owner"`
		Name          string `json:"name"`
		DisplayName   string `json:"displayName"`
		ThemeColor    string `json:"themeColor"`
		HtmlTitle     string `json:"htmlTitle"`
		FaviconUrl    string `json:"faviconUrl"`
		LogoUrl       string `json:"logoUrl"`
		NavbarHtml    string `json:"navbarHtml"`
		FooterHtml    string `json:"footerHtml"`
		StaticBaseUrl string `json:"staticBaseUrl"`
	}{
		Owner:         site.Owner,
		Name:          site.Name,
		DisplayName:   site.DisplayName,
		ThemeColor:    site.ThemeColor,
		HtmlTitle:     site.HtmlTitle,
		FaviconUrl:    site.FaviconUrl,
		LogoUrl:       site.LogoUrl,
		NavbarHtml:    site.NavbarHtml,
		FooterHtml:    site.FooterHtml,
		StaticBaseUrl: site.StaticBaseUrl,
	})
}

func (c *ApiController) UpdateSite() {
	if c.RequireAdmin() {
		return
	}

	id := c.Input().Get("id")

	var site object.Site
	err := json.Unmarshal(c.Ctx.Input.RequestBody, &site)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	success, err := object.UpdateSite(id, &site)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	c.ResponseOk(success)
}

func (c *ApiController) AddSite() {
	if c.RequireAdmin() {
		return
	}

	var site object.Site
	err := json.Unmarshal(c.Ctx.Input.RequestBody, &site)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	success, err := object.AddSite(&site)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	c.ResponseOk(success)
}

func (c *ApiController) DeleteSite() {
	if c.RequireAdmin() {
		return
	}

	var site object.Site
	err := json.Unmarshal(c.Ctx.Input.RequestBody, &site)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	if site.Name == "site-built-in" {
		c.ResponseError("the built-in site cannot be deleted")
		return
	}

	success, err := object.DeleteSite(&site)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	c.ResponseOk(success)
}
