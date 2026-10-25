package opencode

import (
	"context"
	"crypto/sha256"
	"fmt"
	"work2api/internal/core/provider"
)

func (rt *Runtime) CheckAccountIDs() []string {
	ids := []string{}
	seen := map[string]bool{}
	for _, p := range []*nodePool{rt.zenNodes, rt.goNodes} {
		if p != nil {
			for _, n := range p.nodes {
				id := PortalKeyUID(n.key)
				if !seen[id] {
					ids = append(ids, id)
					seen[id] = true
				}
			}
		}
	}
	if rt.cfg.Anonymous {
		ids = append(ids, "anonymous")
	}
	return ids
}
func (rt *Runtime) CheckAccountIdentity(id string) string {
	for _, p := range []*nodePool{rt.zenNodes, rt.goNodes} {
		if p != nil {
			for _, n := range p.nodes {
				if PortalKeyUID(n.key) == id {
					return fmt.Sprintf("%x", sha256.Sum256([]byte(n.key+rt.cfg.Upstream.Zen+rt.cfg.Upstream.Go)))
				}
			}
		}
	}
	if id == "anonymous" && rt.cfg.Anonymous {
		return fmt.Sprintf("%x", sha256.Sum256([]byte(rt.anonymousBase())))
	}
	return ""
}
func (rt *Runtime) CheckAccount(ctx context.Context, id string, force bool) provider.AccountCheck {
	if id == "anonymous" && rt.cfg.Anonymous {
		if rt.transports == nil || len(rt.transports.items) == 0 {
			return provider.Checked("unknown", "出口连通性", "没有可用传输")
		}
		_, status, err := FetchModels(ctx, rt.transports.items[0].client, rt.anonymousBase(), anonymousZenKey)
		if err != nil || status/100 != 2 {
			return provider.Checked("warning", "匿名目录连通性", "目录请求失败；不等同模型调用失败")
		}
		return provider.Checked("unknown", "匿名目录连通性", "目录可达；匿名模型可用性需真实测试")
	}
	for _, tier := range []struct {
		pool *nodePool
		base string
	}{{rt.zenNodes, rt.cfg.Upstream.Zen}, {rt.goNodes, rt.cfg.Upstream.Go}} {
		if tier.pool == nil {
			continue
		}
		for _, node := range tier.pool.nodes {
			if PortalKeyUID(node.key) != id {
				continue
			}
			proxy := tier.pool.Proxy(node)
			if proxy == nil {
				return provider.Checked("warning", "目录连通性", "没有可用出口")
			}
			_, status, err := FetchModels(ctx, proxy.client, tier.base, node.key)
			if err != nil || status/100 != 2 {
				return provider.Checked("warning", "目录接口", "目录请求失败，模型调用授权单独判断")
			}
			return provider.Checked("unknown", "目录连通性", "目录可达，但上游可能公开目录；Key 可用性需真实测试")
		}
	}
	return provider.Checked("unknown", "账号", "未配置个人 Key 或匿名入口")
}
