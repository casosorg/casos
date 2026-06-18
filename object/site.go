package object

import (
	"github.com/casosorg/casos/util"
)

type Site struct {
	Owner       string `xorm:"varchar(100) notnull pk" json:"owner"`
	Name        string `xorm:"varchar(100) notnull pk" json:"name"`
	CreatedTime string `xorm:"varchar(100)" json:"createdTime"`
	DisplayName string `xorm:"varchar(100)" json:"displayName"`

	ThemeColor    string `xorm:"varchar(100)" json:"themeColor"`
	HtmlTitle     string `xorm:"varchar(100)" json:"htmlTitle"`
	FaviconUrl    string `xorm:"varchar(200)" json:"faviconUrl"`
	LogoUrl       string `xorm:"varchar(200)" json:"logoUrl"`
	NavbarHtml    string `xorm:"mediumtext" json:"navbarHtml"`
	FooterHtml    string `xorm:"mediumtext" json:"footerHtml"`
	StaticBaseUrl string `xorm:"varchar(500)" json:"staticBaseUrl"`

	Issuer       string `xorm:"varchar(500)" json:"issuer"`
	ClientId     string `xorm:"varchar(100)" json:"clientId"`
	ClientSecret string `xorm:"varchar(100)" json:"clientSecret"`

	Socks5Proxy string `xorm:"varchar(200)" json:"socks5Proxy"`
	LogConfig   string `xorm:"varchar(1000)" json:"logConfig"`
}

func GetGlobalSites() ([]*Site, error) {
	sites := []*Site{}
	err := ormer.Engine.Asc("owner").Desc("created_time").Find(&sites)
	if err != nil {
		return nil, err
	}
	return sites, nil
}

func GetSite(id string) (*Site, error) {
	owner, name, err := util.GetOwnerAndNameFromIdWithError(id)
	if err != nil {
		return nil, err
	}
	site := &Site{Owner: owner, Name: name}
	existed, err := ormer.Engine.Get(site)
	if err != nil {
		return nil, err
	}
	if !existed {
		return nil, nil
	}
	return site, nil
}

func GetBuiltInSite() (*Site, error) {
	site, err := GetSite("admin/site-built-in")
	if site != nil {
		site.ClientSecret = ""
	}
	return site, err
}

func UpdateSite(id string, site *Site) (bool, error) {
	owner, name, err := util.GetOwnerAndNameFromIdWithError(id)
	if err != nil {
		return false, err
	}
	if s, err := GetSite(id); err != nil {
		return false, err
	} else if s == nil {
		return false, nil
	}
	affected, err2 := ormer.Engine.Where("owner = ? AND name = ?", owner, name).AllCols().Update(site)
	if err2 != nil {
		return false, err2
	}
	return affected != 0, nil
}

func AddSite(site *Site) (bool, error) {
	affected, err := ormer.Engine.Insert(site)
	if err != nil {
		return false, err
	}
	return affected != 0, nil
}

func DeleteSite(site *Site) (bool, error) {
	affected, err := ormer.Engine.Where("owner = ? AND name = ?", site.Owner, site.Name).Delete(&Site{})
	if err != nil {
		return false, err
	}
	return affected != 0, nil
}

func InitSite() {
	site, err := GetBuiltInSite()
	if err != nil || site != nil {
		return
	}
	builtIn := &Site{
		Owner:       "admin",
		Name:        "site-built-in",
		DisplayName: "CasOS",
		HtmlTitle:   "CasOS",
	}
	_, _ = AddSite(builtIn)
}
