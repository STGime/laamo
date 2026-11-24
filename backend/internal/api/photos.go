package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"slices"
	"strings"

	guuid "github.com/google/uuid"

	"github.com/STGime/laamo/backend/internal/bricks/look"
	"github.com/STGime/laamo/backend/internal/catalog"
	"github.com/STGime/laamo/backend/internal/images"
	"github.com/STGime/laamo/backend/internal/store"
)

// uploadedPhoto matches the path UploadPhoto hands out, after
// "images/<workspace>/": in-<random>/p1.jpg.
var uploadedPhoto = regexp.MustCompile(`^in-[0-9a-f-]{36}/p1\.jpg$`)

// UploadPhoto stores a photo from the app's camera button for the helper's
// next run: upright, shrunk and as JPEG, next to the pictures helpers draw.
// StartRun then takes its path. Nothing reads a photo that no run is
// started with.
func (s *Server) UploadPhoto(ctx context.Context, req UploadPhotoRequestObject) (UploadPhotoResponseObject, error) {
	h, ok, err := s.helper(ctx, req.HelperId.String())
	if err != nil {
		return nil, err
	}
	if !ok {
		return UploadPhoto404JSONResponse{NotFoundJSONResponse(helperNotFound)}, nil
	}
	if s.blobs == nil {
		return nil, errors.New("storage not configured")
	}
	raw, err := io.ReadAll(req.Body)
	if err != nil {
		return UploadPhoto400JSONResponse{BadRequestJSONResponse(apiErr(ErrorCodeBadRequest, "the photo is too big; send at most 1 MB"))}, nil
	}
	jpg, err := images.Photo(raw)
	if err != nil {
		return UploadPhoto400JSONResponse{BadRequestJSONResponse(apiErr(ErrorCodeBadRequest, "that isn't a photo we can read (JPEG, PNG or WebP)"))}, nil
	}
	path := photoPath(h.WorkspaceID, "in-"+guuid.NewString(), 1)
	if err := s.blobs.Put(ctx, path, jpg); err != nil {
		return nil, err
	}
	return UploadPhoto201JSONResponse{Path: path}, nil
}

// runPhotos checks the photos a run is started with: each must be an
// upload to this workspace. It returns them as the run's photos value.
func runPhotos(workspaceID string, paths []string) (string, bool) {
	if len(paths) > look.MaxPhotos {
		return "", false
	}
	prefix := "images/" + workspaceID + "/"
	for _, p := range paths {
		rest, ok := strings.CutPrefix(p, prefix)
		if !ok || !uploadedPhoto.MatchString(rest) {
			return "", false
		}
	}
	return strings.Join(paths, ","), true
}

// takesPhotos reports whether a stack reads photos (Read the photo), so
// the app shows the camera button.
func takesPhotos(st catalog.Stack) bool {
	return slices.ContainsFunc(st.Bricks, func(b catalog.StackBrick) bool { return b.Type == "look.read_photo" })
}

// startsWith reports what a run of this stack needs to start: a photo
// (Read the photo), an email (Email this helper), or nothing. With an input
// Run now has nothing to work on, so the app doesn't offer it.
func startsWith(st catalog.Stack) *HelperStartsWith {
	if takesPhotos(st) {
		return new(HelperStartsWithPhoto)
	}
	if slices.ContainsFunc(st.Bricks, func(b catalog.StackBrick) bool { return b.Type == "when.email" }) {
		return new(HelperStartsWithEmail)
	}
	return nil
}

// helperStartsWith is startsWith for a helper's current version.
func (s *Server) helperStartsWith(ctx context.Context, h store.Helper) *HelperStartsWith {
	if h.CurrentVersionID == nil {
		return nil
	}
	v, err := s.store.GetHelperVersion(ctx, *h.CurrentVersionID)
	if err != nil {
		s.log.Warn("helper version", "helper", h.ID, "err", err)
		return nil
	}
	var stack catalog.Stack
	if json.Unmarshal(v.Stack, &stack) != nil {
		return nil
	}
	return startsWith(stack)
}
