package metadata

// extractImageLookupProviderIDs parses image_lookup_provider_ids. It follows
// the same normalization rules as lookup_provider_ids, but opts a provider
// into image requests only; metadata lookup remains controlled separately.
func extractImageLookupProviderIDs(metadataJSON []byte) []string {
	return extractProviderIDKeys(metadataJSON, "image_lookup_provider_ids")
}
