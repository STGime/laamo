package api_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/STGime/laamo/backend/internal/api"
	"github.com/STGime/laamo/backend/internal/catalog"
	"github.com/STGime/laamo/backend/internal/config"
	"github.com/STGime/laamo/backend/internal/engine"
	qmem "github.com/STGime/laamo/backend/internal/queue/memory"
	"github.com/STGime/laamo/backend/internal/store"
	"github.com/STGime/laamo/backend/internal/store/memory"
)

// photo is a JPEG of w×h pixels with soft colour bands.
func photo(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetRGBA(x, y, color.RGBA{uint8(x / 10), uint8(y / 5), uint8((x + y) / 16), 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func isJPEG(b []byte) bool { return bytes.HasPrefix(b, []byte{0xFF, 0xD8}) }

func TestPhotos(t *testing.T) {
	st := memory.New()
	blobs := &engine.MemoryBlobs{}
	cfg := config.Config{InboundDomain: "in.laamo.app", InboundSecret: secret}
	srv := httptest.NewServer(api.NewServer(api.Deps{Config: cfg, Store: st, Auth: fakeAuth{"anna": anna, "ben": ben},
		Queue: qmem.New(), Blobs: blobs}).Handler())
	t.Cleanup(srv.Close)
	e := env{url: srv.URL, store: st}
	ctx := context.Background()
	ws := st.PutWorkspace(store.Workspace{UserID: anna.ID, Name: "Bäckerei", Locale: "de", Timezone: "Europe/Berlin"})
	st.PutMember(store.Member{WorkspaceID: ws.ID, UserID: anna.ID, Role: store.RoleOwner})
	h := st.PutHelper(store.Helper{WorkspaceID: ws.ID, UserID: anna.ID, Name: "Belege", Status: "active"})
	raw, _ := json.Marshal(catalog.Stack{Schema: 1, Bricks: []catalog.StackBrick{
		{ID: "b1", Type: "when.email"}, {ID: "b2", Type: "look.read_photo"}}})
	v := st.PutHelperVersion(store.HelperVersion{WorkspaceID: ws.ID, UserID: anna.ID, HelperID: h.ID, Version: 1, Stack: raw})
	h.CurrentVersionID = &v.ID
	st.PutHelper(h)
	c := e.client(t, "anna")

	// A helper with Read the photo says so: the app shows the camera button.
	if d := must(c.GetHelperWithResponse(ctx, uuidOf(h.ID))); d.JSON200 == nil || d.JSON200.TakesPhotos == nil || !*d.JSON200.TakesPhotos {
		t.Fatalf("takes_photos missing: %s", d.Body)
	}

	// The camera button: upload, then start the run with the path.
	up := must(c.UploadPhotoWithBodyWithResponse(ctx, uuidOf(h.ID), "image/jpeg", bytes.NewReader(photo(t, 2400, 1200))))
	if up.StatusCode() != 201 || up.JSON201 == nil || !strings.HasPrefix(up.JSON201.Path, "images/"+ws.ID+"/in-") {
		t.Fatalf("upload: %d %s", up.StatusCode(), up.Body)
	}
	path := up.JSON201.Path
	stored, err := blobs.Get(ctx, path)
	if err != nil || !isJPEG(stored) {
		t.Fatalf("stored photo: %v", err)
	}
	if cfgImg, err := jpeg.DecodeConfig(bytes.NewReader(stored)); err != nil || cfgImg.Width != 1800 || cfgImg.Height != 900 {
		t.Fatalf("stored size %d×%d, want it shrunk to 1800×900 (%v)", cfgImg.Width, cfgImg.Height, err)
	}
	// It's served like a drawn picture, to members only.
	if img := must(c.GetImageWithResponse(ctx, &api.GetImageParams{Path: path})); img.StatusCode() != 200 || !isJPEG(img.Body) {
		t.Fatalf("get image: %d", img.StatusCode())
	}
	if img := must(e.client(t, "ben").GetImageWithResponse(ctx, &api.GetImageParams{Path: path})); img.StatusCode() != 404 {
		t.Fatalf("a stranger got the photo: %d", img.StatusCode())
	}

	// Only this workspace's uploads can start a run: not another
	// workspace's, not a drawn picture, not any path.
	for _, bad := range []string{
		"images/" + store.NewID() + "/in-" + store.NewID() + "/p1.jpg",
		"images/" + ws.ID + "/" + store.NewID() + "/c1.jpg",
		"mail/" + ws.ID + "/x.eml",
	} {
		if r := must(c.StartRunWithResponse(ctx, uuidOf(h.ID), api.StartRunJSONRequestBody{Photos: &[]string{bad}})); r.StatusCode() != 400 {
			t.Errorf("start with %s: %d, want 400", bad, r.StatusCode())
		}
	}
	run := must(c.StartRunWithResponse(ctx, uuidOf(h.ID), api.StartRunJSONRequestBody{Photos: &[]string{path}}))
	if run.StatusCode() != 202 {
		t.Fatalf("start: %d %s", run.StatusCode(), run.Body)
	}
	r, err := st.GetRun(ctx, run.JSON202.RunId.String())
	if err != nil || r.TriggerData["photos"] != path || r.TriggerType != "manual" {
		t.Fatalf("run: %+v, %v", r.TriggerData, err)
	}

	// Not a picture, and not your helper.
	if r := must(c.UploadPhotoWithBodyWithResponse(ctx, uuidOf(h.ID), "image/jpeg", strings.NewReader("%PDF-1.7"))); r.StatusCode() != 400 {
		t.Errorf("upload of a PDF: %d, want 400", r.StatusCode())
	}
	if r := must(e.client(t, "ben").UploadPhotoWithBodyWithResponse(ctx, uuidOf(h.ID), "image/jpeg", bytes.NewReader(photo(t, 40, 40)))); r.StatusCode() != 404 {
		t.Errorf("a stranger's upload: %d, want 404", r.StatusCode())
	}

	// By email: the pictures attached to the mail come with the run.
	got := must(c.GetHelperStackWithResponse(ctx, uuidOf(h.ID))).JSON200.Stack
	got.Bricks[0].Config = map[string]any{"senders": []any{}} // anyone may write
	saved := must(c.SaveHelperStackWithResponse(ctx, uuidOf(h.ID), api.SaveStack{Stack: got, Version: 1}))
	if saved.StatusCode() != 200 || saved.JSON200.EmailAddress == nil {
		t.Fatalf("save: %d %s", saved.StatusCode(), saved.Body)
	}
	b64 := base64.StdEncoding.EncodeToString(photo(t, 300, 200))
	mail := "From: Anna <anna@example.com>\r\nTo: x@in.laamo.app\r\nSubject: Beleg\r\nMessage-ID: <beleg@example.com>\r\n" +
		"MIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=XX\r\n\r\n" +
		"--XX\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nAnbei der Beleg.\r\n" +
		"--XX\r\nContent-Type: image/jpeg\r\nContent-Disposition: attachment; filename=\"IMG_4411.jpg\"\r\nContent-Transfer-Encoding: base64\r\n\r\n" + b64 + "\r\n" +
		"--XX\r\nContent-Type: application/pdf\r\nContent-Disposition: attachment; filename=\"Rechnung.pdf\"\r\n\r\n%PDF-1.7\r\n" +
		"--XX--\r\n"
	if code, out := deliver(t, srv.URL, "<"+*saved.JSON200.EmailAddress+">", []byte(mail), secret); code != 202 || out != "run_started" {
		t.Fatalf("deliver: %d %s", code, out)
	}
	runs, _ := st.ListRuns(ctx, store.RunFilter{HelperID: h.ID, Limit: 10})
	var mailRun *store.Run
	for i := range runs {
		if runs[i].TriggerType == store.TriggerEmail {
			mailRun = &runs[i]
		}
	}
	if mailRun == nil {
		t.Fatal("no email run")
	}
	want := "images/" + ws.ID + "/" + mailRun.ID + "/p1.jpg"
	if mailRun.TriggerData["photos"] != want || mailRun.TriggerData["attachments"] != "IMG_4411.jpg, Rechnung.pdf" {
		t.Fatalf("email run: %+v", mailRun.TriggerData)
	}
	if data, err := blobs.Get(ctx, want); err != nil || !isJPEG(data) {
		t.Fatalf("the mail's photo isn't stored: %v", err)
	}
}
