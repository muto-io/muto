"""Minimal GitHub REST client (stdlib only) with Link-header pagination."""

from __future__ import annotations

import json
import re
import urllib.error
import urllib.parse
import urllib.request

API_ROOT = "https://api.github.com"
_NEXT = re.compile(r'<([^>]+)>;\s*rel="next"')


class GitHubError(Exception):
    """Any failure talking to the GitHub API."""


def next_link(link_header):
    """Return the rel="next" URL from a Link header, or None."""
    if not link_header:
        return None
    match = _NEXT.search(link_header)
    return match.group(1) if match else None


class GitHubClient:
    def __init__(self, token=None, api_root=API_ROOT, timeout=30):
        self.token = token
        self.api_root = api_root
        self.timeout = timeout

    def _request(self, url):
        """Fetch `url`; return (parsed_json, headers). The single network seam."""
        headers = {
            "Accept": "application/vnd.github+json",
            "X-GitHub-Api-Version": "2022-11-28",
            "User-Agent": "muto-ci-health",
        }
        if self.token:
            headers["Authorization"] = f"Bearer {self.token}"
        request = urllib.request.Request(url, headers=headers)
        try:
            with urllib.request.urlopen(request, timeout=self.timeout) as response:
                return json.load(response), response.headers
        except urllib.error.HTTPError as exc:
            raise GitHubError(f"GET {url} failed: HTTP {exc.code}") from exc
        except (urllib.error.URLError, TimeoutError, json.JSONDecodeError) as exc:
            raise GitHubError(f"GET {url} failed: {exc}") from exc

    def _url(self, path, params):
        query = urllib.parse.urlencode(params or {})
        return f"{self.api_root}{path}" + (f"?{query}" if query else "")

    def get(self, path, params=None):
        body, _ = self._request(self._url(path, params))
        return body

    def paginate(self, path, params=None, key=None, max_pages=20):
        """Yield items across pages. `key` names the list inside object responses."""
        params = {"per_page": 100, **(params or {})}
        url = self._url(path, params)
        for _ in range(max_pages):
            body, headers = self._request(url)
            yield from (body[key] if key else body)
            url = next_link(headers.get("Link"))
            if not url:
                return
