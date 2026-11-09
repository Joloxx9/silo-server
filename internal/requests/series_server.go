package requests

import "context"

// seriesServerConnected reports whether an enabled download server
// connection takes series requests. Silo records no live health for these
// connections, so enabled is the best it can say.
func (s *Service) seriesServerConnected(ctx context.Context) (bool, error) {
	fc, err := s.newFulfillContext(ctx)
	if err != nil {
		return false, err
	}
	return len(seriesRouterConnections(fc)) > 0, nil
}
