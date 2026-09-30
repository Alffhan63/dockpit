package docker

import "testing"

func TestMaskEnv(t *testing.T) {
	masked := []string{
		"DB_PASSWORD=hunter2", "MYSQL_ROOT_PASSWORD=x", "API_KEY=abc", "JWT_SECRET=s", "GITHUB_TOKEN=t",
		"AWS_SECRET_ACCESS_KEY=k", "SESSION_SALT=z", "SENTRY_DSN=https://a@b/1", "PRIVATE_KEY=zzz",
	}
	for _, kv := range masked {
		if e := maskEnv(kv); !e.Masked || e.Value != maskedValue {
			t.Errorf("maskEnv(%q) = %+v, want masked", kv, e)
		}
	}
	plain := map[string]string{"NODE_ENV=production": "production", "PORT=3000": "3000", "TZ=Asia/Jakarta": "Asia/Jakarta", "KEYBOARD=us": "us"}
	for kv, want := range plain {
		if e := maskEnv(kv); e.Masked || e.Value != want {
			t.Errorf("maskEnv(%q) = %+v, want plain %q", kv, e, want)
		}
	}
	e := maskEnv("DATABASE_URL=postgres://app:s3cret@db:5432/x")
	if !e.Masked || e.Value != "postgres://app:"+maskedValue+"@db:5432/x" {
		t.Errorf("url creds not masked: %+v", e)
	}
	if e := maskEnv("EMPTY_PASSWORD="); e.Masked {
		t.Errorf("empty value should not be masked: %+v", e)
	}
}

func TestHealthFromStatus(t *testing.T) {
	for status, want := range map[string]string{
		"Up 3 hours (healthy)": "healthy", "Up 3 hours (unhealthy)": "unhealthy",
		"Up 5 seconds (health: starting)": "starting", "Up 3 hours": "", "Exited (1) 2 minutes ago": "",
	} {
		if got := healthFromStatus(status); got != want {
			t.Errorf("healthFromStatus(%q) = %q, want %q", status, got, want)
		}
	}
}
