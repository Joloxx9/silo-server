package newsletter

func (env RenderEnv) link(id string) string { return itemURL(env.PublicURL, id) }
