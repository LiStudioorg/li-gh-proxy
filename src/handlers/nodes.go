package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"li-gh-proxy/config"
)

// RegisterNodesRoute 注册加速节点 API。
// 节点列表来自 config.GetConfig()（重启后生效），供前端展示与切换；
// 未配置节点时返回空列表，前端回退为当前站点域名。
func RegisterNodesRoute(r *gin.Engine) {
	r.GET("/api/nodes", func(c *gin.Context) {
		cfg := config.GetConfig()

		nodes := make([]config.NodeConfig, len(cfg.Nodes))
		copy(nodes, cfg.Nodes)

		// 节点列表跟随配置文件，禁止任何层缓存，避免配置更新后读到过期数据
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{
			"current": c.Request.Host,
			"nodes":   nodes,
		})
	})
}
