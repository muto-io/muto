import http.client
import unittest
from unittest import mock

import ghapi


class NextLinkTest(unittest.TestCase):
    def test_extracts_next(self):
        header = (
            '<https://api.github.com/x?page=2>; rel="next", '
            '<https://api.github.com/x?page=5>; rel="last"'
        )
        self.assertEqual(ghapi.next_link(header), "https://api.github.com/x?page=2")

    def test_missing_or_last_page(self):
        self.assertIsNone(ghapi.next_link(None))
        self.assertIsNone(ghapi.next_link('<https://api.github.com/x?page=1>; rel="prev"'))


class FakeClient(ghapi.GitHubClient):
    def __init__(self, pages):
        super().__init__(token=None, api_root="https://example.test")
        self.pages = pages
        self.urls = []

    def _request(self, url):
        self.urls.append(url)
        body, link = self.pages[len(self.urls) - 1]
        return body, {"Link": link} if link else {}


class PaginateTest(unittest.TestCase):
    def test_follows_next_links_and_unwraps_key(self):
        client = FakeClient(
            [
                ({"runs": [1, 2]}, '<https://example.test/p2>; rel="next"'),
                ({"runs": [3]}, None),
            ]
        )
        self.assertEqual(list(client.paginate("/runs", {"status": "completed"}, key="runs")), [1, 2, 3])
        self.assertEqual(client.urls[0], "https://example.test/runs?per_page=100&status=completed")
        self.assertEqual(client.urls[1], "https://example.test/p2")

    def test_plain_list_response_without_key(self):
        client = FakeClient([([{"n": 1}], None)])
        self.assertEqual(list(client.paginate("/pulls")), [{"n": 1}])

    def test_stops_at_max_pages(self):
        link = '<https://example.test/again>; rel="next"'
        client = FakeClient([([1], link)] * 5)
        self.assertEqual(list(client.paginate("/x", max_pages=3)), [1, 1, 1])


class RequestErrorWrappingTest(unittest.TestCase):
    def _assert_wrapped(self, exc):
        with mock.patch("urllib.request.urlopen", side_effect=exc):
            with self.assertRaises(ghapi.GitHubError):
                ghapi.GitHubClient()._request("https://example.test/x")

    def test_connection_reset_is_wrapped(self):
        self._assert_wrapped(ConnectionResetError("reset"))

    def test_incomplete_read_is_wrapped(self):
        self._assert_wrapped(http.client.IncompleteRead(b""))


if __name__ == "__main__":
    unittest.main()
