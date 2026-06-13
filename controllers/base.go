package controllers

import "github.com/beego/beego"

type Response struct {
	Status string      `json:"status"`
	Msg    string      `json:"msg"`
	Data   interface{} `json:"data"`
}

type ApiController struct {
	beego.Controller
}

func (c *ApiController) ResponseOk(data ...interface{}) {
	resp := &Response{Status: "ok"}
	if len(data) > 0 {
		resp.Data = data[0]
	}
	c.Data["json"] = resp
	c.ServeJSON()
}

func (c *ApiController) ResponseError(error string, data ...interface{}) {
	resp := &Response{Status: "error", Msg: error}
	if len(data) > 0 {
		resp.Data = data[0]
	}
	c.Data["json"] = resp
	c.ServeJSON()
}
