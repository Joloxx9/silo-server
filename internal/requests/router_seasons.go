package requests

import (
	"context"
	"fmt"
)

// RouterFeatures are the optional request_router.v1 features a router
// capability declares in its plugin manifest.
type RouterFeatures struct {
	// SupportsSeasons: Fulfill and CheckStatus honor a series request's
	// seasons. Such a plugin acquires only the requested seasons and adds
	// them to a series its download server already has, leaving the other
	// seasons alone. Any other plugin takes the whole series.
	SupportsSeasons bool
}

// RouterFeatureReader reads a router capability's declared features from the
// manifest the host stored at install, without launching the plugin. A
// RequestRouterProvider that does not implement it declares no features.
type RouterFeatureReader interface {
	RouterFeatures(ctx context.Context, installationID int, capabilityID string) (RouterFeatures, error)
}

// routerCapabilityKey names one plugin installation's router capability.
type routerCapabilityKey struct {
	installationID int
	capabilityID   string
}

// routerSupportsSeasons reports whether the router capability takes a request
// for particular seasons. The answer is kept for the fulfill context's
// lifetime, one reconcile pass or one request.
func (s *Service) routerSupportsSeasons(ctx context.Context, fc *fulfillContext, installationID int, capabilityID string) (bool, error) {
	reader, ok := s.router.(RouterFeatureReader)
	if !ok {
		return false, nil
	}
	key := routerCapabilityKey{installationID, capabilityID}
	fc.mu.Lock()
	supported, cached := fc.seasonSupport[key]
	fc.mu.Unlock()
	if cached {
		return supported, nil
	}
	features, err := reader.RouterFeatures(ctx, installationID, capabilityID)
	if err != nil {
		return false, fmt.Errorf("read request router features: %w", err)
	}
	fc.mu.Lock()
	if fc.seasonSupport == nil {
		fc.seasonSupport = map[routerCapabilityKey]bool{}
	}
	fc.seasonSupport[key] = features.SupportsSeasons
	fc.mu.Unlock()
	return features.SupportsSeasons, nil
}

// allTakeSeasons reports whether every given connection is bound to a router
// capability that takes seasons. A connection bound to none cannot.
func (s *Service) allTakeSeasons(ctx context.Context, fc *fulfillContext, conns []Integration) (bool, error) {
	for _, in := range conns {
		if in.InstallationID == nil || in.CapabilityID == "" {
			return false, nil
		}
		ok, err := s.routerSupportsSeasons(ctx, fc, *in.InstallationID, in.CapabilityID)
		if err != nil || !ok {
			return false, err
		}
	}
	return true, nil
}

// seriesRouterConnections returns the enabled connections meant to serve
// series, the ones routerConfiguredFor counts, including misconfigured ones.
func seriesRouterConnections(fc *fulfillContext) []Integration {
	var out []Integration
	for _, in := range fc.integrations {
		if in.Enabled && in.CapabilityID != "" && integrationSupportsMediaType(in, MediaTypeSeries) {
			out = append(out, in)
		}
	}
	return out
}

// integrationsByID returns the connections with the given ids that still
// exist. A route to a removed server fails at submission instead.
func integrationsByID(fc *fulfillContext, ids []string) []Integration {
	var out []Integration
	for _, id := range ids {
		for _, in := range fc.integrations {
			if in.ID == id {
				out = append(out, in)
				break
			}
		}
	}
	return out
}

// missingSeasonsDeliverable reports whether a request for seasons of a series
// already in the library can go to a download server: only a router that
// takes seasons fetches just those, and any other would add the whole series
// again. The server a request goes to decides.
//
// Without routing rules the plugin picks among the series connections, so all
// of them must take seasons. With rules, the servers the rules choose for this
// title must; before its routing facts are captured that is every server a
// series rule sends to. submitRouted checks the chosen server again, since
// facts read after the claim can choose another.
func (s *Service) missingSeasonsDeliverable(ctx context.Context, fc *fulfillContext, req Request) (bool, error) {
	routes := fc.routesFor(MediaTypeSeries)
	if len(routes) == 0 {
		return s.allTakeSeasons(ctx, fc, seriesRouterConnections(fc))
	}
	var ids []string
	if req.RoutingFacts.Captured() {
		allowed, _ := s.allowedQualities(ctx, req, fc.settings)
		for _, decision := range decideRoutes(routes, req, allowed) {
			if !decision.Skip {
				ids = append(ids, decision.IntegrationID)
			}
		}
	} else {
		for _, route := range routes {
			if route.MediaType != MediaTypeSeries || !route.Enabled {
				continue
			}
			for _, id := range []string{route.HD.IntegrationID, route.UHD.IntegrationID} {
				if id != "" {
					ids = append(ids, id)
				}
			}
		}
	}
	return s.allTakeSeasons(ctx, fc, integrationsByID(fc, ids))
}

// errMissingSeasonsUnsupported is a submission error for a request for the
// missing seasons of a series in the library whose chosen server's plugin
// would add the whole series.
func errMissingSeasonsUnsupported(server string) error {
	return fmt.Errorf("%s cannot fetch only the missing seasons of a series already in the library: its plugin would add the whole series", server)
}

// integrationName is a connection's display name, or its id when it is gone.
func integrationName(fc *fulfillContext, id string) string {
	for _, in := range fc.integrations {
		if in.ID == id {
			return in.Name
		}
	}
	return id
}
