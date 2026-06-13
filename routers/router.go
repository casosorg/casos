package routers

import (
	"github.com/beego/beego"
	"github.com/casosorg/casos/controllers"
)

func InitAPI() {
	beego.Router("/api/get-pods", &controllers.ApiController{}, "GET:GetPods")

	beego.Router("/api/get-configmaps", &controllers.ApiController{}, "GET:GetConfigMaps")
	beego.Router("/api/get-configmap", &controllers.ApiController{}, "GET:GetConfigMap")
	beego.Router("/api/add-configmap", &controllers.ApiController{}, "POST:AddConfigMap")
	beego.Router("/api/update-configmap", &controllers.ApiController{}, "POST:UpdateConfigMap")
	beego.Router("/api/delete-configmap", &controllers.ApiController{}, "POST:DeleteConfigMap")
}
