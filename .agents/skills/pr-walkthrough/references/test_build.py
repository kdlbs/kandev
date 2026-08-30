"""Unit tests for build.py.

Run from this directory with:
    python3 -m unittest test_build

These tests guard the parts of the renderer a bad JSON file could break
silently: the raw-patch to Shiki-marker conversion, per-language comment
tokens, block rendering and its mutual-exclusion rules, HTML escaping,
GitHub file links, canvas layout, edge validation, and the required-field
checks. build.py uses only the standard library, and so do these tests.
"""

import os
import sys
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)

import build  # noqa: E402


def minimal_change():
    """A fresh, valid change dict. layout() mutates it, so never share one."""
    return {
        "title": "T",
        "file": "a/b.go",
        "why": "does a thing",
        "blocks": [{"lang": "go", "code": "x := 1"}],
    }


def minimal_data():
    return {
        "pr": {"number": 1, "title": "P", "url": "https://h/pr/1",
               "repo": "o/r", "base": "master", "head": "feat"},
        "why": {"problem": "p", "what": ["w"]},
        "changes": [minimal_change()],
        "risk": {"score": 2, "reasons": ["r"]},
        "review": {},
    }


class TestPatchToMarked(unittest.TestCase):
    def test_headers_dropped_context_kept_indices_recorded(self):
        patch = ("diff --git a/f b/f\nindex 1..2 100644\n--- a/f\n+++ b/f\n"
                 "@@ -1,3 +1,3 @@ func F() {\n ctx\n-old\n+new\n more")
        code, added, removed = build.patch_to_marked(patch)
        self.assertEqual(code, "ctx\nold\nnew\nmore")
        self.assertEqual(added, [3])
        self.assertEqual(removed, [2])
        for h in ("diff --git", "index ", "@@", "--- ", "+++ ", "[!code"):
            self.assertNotIn(h, code)

    def test_blank_line_preserved(self):
        self.assertEqual(build.patch_to_marked(" a\n\n b"), ("a\n\nb", [], []))

    def test_marker_never_inlined(self):
        code, added, removed = build.patch_to_marked("+x\n-y")
        self.assertEqual(code, "x\ny")
        self.assertEqual((added, removed), ([1], [2]))

    def test_no_newline_marker_dropped(self):
        code, added, removed = build.patch_to_marked(
            "@@ -1 +1 @@\n-old\n+new\n\\ No newline at end of file")
        self.assertEqual(code, "old\nnew")
        self.assertEqual((added, removed), ([2], [1]))

    def test_removed_line_starting_with_double_dash_is_kept(self):
        # Inside a hunk, a removed "-- note" line becomes "--- note" in the
        # patch (leading '-' diff marker). It must be kept as a removed line,
        # not dropped as a header. The diff marker is stripped, leaving "-- note".
        patch = "@@ -1,1 +1,0 @@\n--- note"
        code, added, removed = build.patch_to_marked(patch)
        self.assertEqual(code, "-- note")
        self.assertEqual((added, removed), ([], [1]))

    def test_added_line_starting_with_double_plus_is_kept(self):
        # Inside a hunk, an added "++i" line becomes "+++i" in the patch. It
        # must be kept as an added line, not dropped as a header.
        patch = "@@ -1,0 +1,1 @@\n+++i"
        code, added, removed = build.patch_to_marked(patch)
        self.assertEqual(code, "++i")
        self.assertEqual((added, removed), ([1], []))

    def test_file_headers_before_first_hunk_still_dropped(self):
        patch = ("diff --git a/f b/f\nindex 1..2 100644\n--- a/f\n+++ b/f\n"
                 "@@ -1,1 +1,1 @@\n-old\n+new")
        code, added, removed = build.patch_to_marked(patch)
        self.assertEqual(code, "old\nnew")
        self.assertEqual((added, removed), ([2], [1]))

    def test_comment_token_inside_string_is_not_treated_as_marker(self):
        # A Rust raw string that contains "//" must stay intact: the marker is
        # never inlined, so nothing inside the code can look like a diff marker.
        patch = '@@ -1,0 +1,1 @@\n+    r#"{"url":"/x"}, // note"#'
        code, added, removed = build.patch_to_marked(patch)
        self.assertEqual(code, '    r#"{"url":"/x"}, // note"#')
        self.assertEqual((added, removed), ([1], []))
        self.assertNotIn("[!code", code)


class TestCommentTokens(unittest.TestCase):
    def test_known_languages(self):
        self.assertEqual(build.COMMENT_TOKENS["python"], "#")
        self.assertEqual(build.COMMENT_TOKENS["bash"], "#")
        self.assertEqual(build.COMMENT_TOKENS["sql"], "--")
        self.assertEqual(build.COMMENT_TOKENS["clojure"], ";")

    def test_unknown_language_falls_back_to_slashes(self):
        self.assertEqual(build.COMMENT_TOKENS.get("go", "//"), "//")


class TestRenderBlock(unittest.TestCase):
    def test_plain_excerpt_escapes_and_has_no_diff(self):
        html = build.render_block({"lang": "go", "code": "a < b & c"}, "c")
        self.assertIn("a &lt; b &amp; c", html)
        self.assertNotIn('data-diff="true"', html)

    def test_hand_marked_diff_strips_markers_and_records_indices(self):
        html = build.render_block(
            {"lang": "go", "code": "old // [!code --]\nnew // [!code ++]",
             "diff": True}, "c")
        self.assertIn('data-diff="true"', html)
        # The markers are stripped from the code and turned into index attrs.
        self.assertNotIn("[!code", html)
        self.assertIn('data-added="2"', html)
        self.assertIn('data-removed="1"', html)
        # The bare code text (marker removed) is still present, escaped.
        self.assertIn("old\nnew", html)

    def test_diff_with_no_markers_marks_all_lines_added(self):
        html = build.render_block(
            {"lang": "python", "code": "def f():\n\n    return 1",
             "diff": True}, "c")
        self.assertIn('data-diff="true"', html)
        # Every line is added, including the blank line 2, so the green tint
        # stays continuous across the excerpt.
        self.assertIn('data-added="1,2,3"', html)
        self.assertNotIn("data-removed", html)
        self.assertNotIn("[!code", html)

    def test_all_added_indices_includes_blank_lines(self):
        self.assertEqual(build.all_added_indices("a\n\nb"), [1, 2, 3])

    def test_parse_hand_marked_strips_any_comment_token(self):
        code, added, removed = build.parse_hand_marked(
            "keep\nx = 1 # [!code ++]\ndrop -- [!code --]")
        self.assertEqual(code, "keep\nx = 1\ndrop")
        self.assertEqual((added, removed), ([2], [3]))

    def test_markdown_block(self):
        html = build.render_block(
            {"render": "markdown", "code": "| a |"}, "c")
        self.assertIn('data-render="markdown"', html)

    def test_patch_block_marks_diff_by_index(self):
        html = build.render_block(
            {"lang": "bash", "patch": "+echo hi"}, "c")
        self.assertIn('data-diff="true"', html)
        self.assertIn('data-added="1"', html)
        self.assertNotIn("[!code", html)

    def test_patch_and_code_together_fails(self):
        with self.assertRaises(build.BuildError):
            build.render_block(
                {"lang": "go", "patch": "+x", "code": "x"}, "c")

    def test_patch_with_diff_flag_fails(self):
        with self.assertRaises(build.BuildError):
            build.render_block(
                {"lang": "go", "patch": "+x", "diff": True}, "c")

    def test_missing_lang_fails(self):
        with self.assertRaises(build.BuildError):
            build.render_block({"code": "x"}, "c")

    def test_block_risk_emits_data_attr(self):
        html = build.render_block(
            {"lang": "go", "code": "x := 1",
             "risk": {"score": 7,
                      "lines": [{"match": "x", "severity": "high",
                                 "note": "n"}]}},
            "c")
        self.assertIn("data-risk=", html)
        self.assertIn("&quot;match&quot;: &quot;x&quot;", html)
        self.assertIn("&quot;severity&quot;: &quot;high&quot;", html)

    def test_block_risk_without_lines_emits_no_data_attr(self):
        html = build.render_block(
            {"lang": "go", "code": "x := 1", "risk": {"score": 4}}, "c")
        self.assertNotIn("data-risk=", html)


class TestBlockRiskChip(unittest.TestCase):
    def test_chip_carries_score_and_reason(self):
        chip = build.block_risk_chip({"score": 7, "reason": "spins"})
        self.assertIn('data-score="7"', chip)
        self.assertIn('title="spins"', chip)
        self.assertIn('block-risk-score">7<', chip)

    def test_chip_empty_without_score(self):
        self.assertEqual(build.block_risk_chip({"reason": "x"}), "")


class TestValidateBlockRisk(unittest.TestCase):
    def test_valid_risk_passes(self):
        risk = {"score": 5,
                "lines": [{"match": "y", "severity": "medium"}]}
        self.assertIs(build.validate_block_risk(risk, "c"), risk)

    def test_score_out_of_range_fails(self):
        with self.assertRaises(build.BuildError):
            build.validate_block_risk({"score": 11}, "c")

    def test_line_missing_match_fails(self):
        with self.assertRaises(build.BuildError):
            build.validate_block_risk(
                {"lines": [{"severity": "high"}]}, "c")

    def test_bad_severity_fails(self):
        with self.assertRaises(build.BuildError):
            build.validate_block_risk(
                {"lines": [{"match": "y", "severity": "critical"}]}, "c")

    def test_validate_catches_bad_block_risk(self):
        data = minimal_data()
        data["changes"][0]["blocks"][0]["risk"] = {"score": 99}
        with self.assertRaises(build.BuildError):
            build.validate(data)

    def test_missing_pr_repo_fails(self):
        data = minimal_data()
        del data["pr"]["repo"]
        with self.assertRaises(build.BuildError):
            build.validate(data)

    def test_why_what_string_fails(self):
        data = minimal_data()
        data["why"]["what"] = "one string not a list"
        with self.assertRaises(build.BuildError):
            build.validate(data)

    def test_risk_reasons_string_fails(self):
        data = minimal_data()
        data["risk"]["reasons"] = "one string not a list"
        with self.assertRaises(build.BuildError):
            build.validate(data)

    def test_top_level_risk_score_non_numeric_fails(self):
        data = minimal_data()
        data["risk"]["score"] = "high"
        with self.assertRaises(build.BuildError):
            build.validate(data)

    def test_top_level_risk_score_out_of_range_fails(self):
        data = minimal_data()
        data["risk"]["score"] = 11
        with self.assertRaises(build.BuildError):
            build.validate(data)


class TestEscaping(unittest.TestCase):
    def test_esc_leaves_quotes_but_escapes_angles(self):
        self.assertEqual(build.esc('a<b>"c'), 'a&lt;b&gt;"c')

    def test_esc_attr_escapes_quotes(self):
        self.assertIn("&quot;", build.esc_attr('a"b'))

    def test_none_becomes_empty(self):
        self.assertEqual(build.esc(None), "")


class TestFileUrl(unittest.TestCase):
    def test_explicit_url_wins(self):
        pr = {"url": "https://h/pr/1"}
        self.assertEqual(
            build.file_url({"file": "x", "file_url": "https://o"}, pr),
            "https://o")

    def test_default_is_sha_anchor(self):
        url = build.file_url({"file": "a/b.go"}, {"url": "https://h/pr/1"})
        self.assertTrue(url.startswith("https://h/pr/1/files#diff-"))
        self.assertEqual(len(url.split("diff-")[1]), 64)

    def test_executable_scheme_is_rejected(self):
        with self.assertRaises(build.BuildError):
            build.file_url(
                {"file": "x", "file_url": "javascript:alert(1)"},
                {"url": "https://h/pr/1"})

    def test_non_http_scheme_is_rejected(self):
        with self.assertRaises(build.BuildError):
            build.file_url(
                {"file": "x", "file_url": "data:text/html,payload"},
                {"url": "https://h/pr/1"})


class TestLayout(unittest.TestCase):
    def test_ids_assigned_and_two_column_grid(self):
        changes = [minimal_change() for _ in range(3)]
        boxes = build.layout(changes)
        self.assertEqual([c["id"] for c in changes], ["n1", "n2", "n3"])
        self.assertEqual(boxes, [])
        # 3 changes -> 2 columns, so n3 wraps to the second row, column 0.
        self.assertEqual(changes[2]["_x"], build.ORIGIN_X)
        self.assertEqual(changes[2]["_y"], build.ORIGIN_Y + build.ROW_STRIDE)

    def test_more_than_four_changes_uses_three_columns(self):
        changes = [minimal_change() for _ in range(5)]
        build.layout(changes)
        self.assertEqual(changes[3]["_x"], build.ORIGIN_X + 0 * build.COL_STRIDE)
        self.assertEqual(changes[3]["_y"], build.ORIGIN_Y + build.ROW_STRIDE)

    def test_groups_make_one_column_each_with_a_box(self):
        changes = [minimal_change(), minimal_change(), minimal_change()]
        changes[0]["group"] = "A"
        changes[1]["group"] = "A"
        changes[2]["group"] = "B"
        boxes = build.layout(changes)
        self.assertEqual(len(boxes), 2)
        # Group A is column 0, its two nodes stack in rows 0 and 1.
        self.assertEqual(changes[0]["_x"], build.ORIGIN_X)
        self.assertEqual(changes[1]["_y"], build.ORIGIN_Y + build.ROW_STRIDE)
        # Group B is column 1.
        self.assertEqual(changes[2]["_x"], build.ORIGIN_X + build.COL_STRIDE)


class TestRenderEdges(unittest.TestCase):
    def test_valid_edge_renders(self):
        html = build.render_edges(
            [{"from": "n1", "to": "n2", "label": "calls"}], {"n1", "n2"})
        self.assertIn('data-from="n1"', html)
        self.assertIn('data-label="calls"', html)

    def test_shared_endpoints_receive_distinct_route_lanes(self):
        html = build.render_edges(
            [
                {"from": "n1", "to": "n2", "label": "first"},
                {"from": "n1", "to": "n3", "label": "second"},
                {"from": "n2", "to": "n4", "label": "third"},
                {"from": "n3", "to": "n4", "label": "fourth"},
            ],
            {"n1", "n2", "n3", "n4"},
        )
        self.assertIn(
            'data-from-lane="0" data-from-lanes="2" '
            'data-to-lane="0" data-to-lanes="1"',
            html,
        )
        self.assertIn(
            'data-from-lane="1" data-from-lanes="2" '
            'data-to-lane="0" data-to-lanes="1"',
            html,
        )
        self.assertIn(
            'data-from-lane="0" data-from-lanes="1" '
            'data-to-lane="0" data-to-lanes="2"',
            html,
        )
        self.assertIn(
            'data-from-lane="0" data-from-lanes="1" '
            'data-to-lane="1" data-to-lanes="2"',
            html,
        )

    def test_reciprocal_edges_receive_distinct_pair_lanes(self):
        html = build.render_edges(
            [
                {"from": "n1", "to": "n2", "label": "forward"},
                {"from": "n2", "to": "n1", "label": "backward"},
            ],
            {"n1", "n2"},
        )
        self.assertIn(
            'data-pair-lane="0" data-pair-lanes="2"',
            html,
        )
        self.assertIn(
            'data-pair-lane="1" data-pair-lanes="2"',
            html,
        )

    def test_unknown_from_node_fails(self):
        with self.assertRaises(build.BuildError):
            build.render_edges([{"from": "nX", "to": "n1"}], {"n1"})

    def test_unknown_to_node_fails(self):
        with self.assertRaises(build.BuildError):
            build.render_edges([{"from": "n1", "to": "nX"}], {"n1"})


class TestValidate(unittest.TestCase):
    def test_minimal_data_passes(self):
        build.validate(minimal_data())  # no raise

    def test_missing_pr_field_fails(self):
        data = minimal_data()
        del data["pr"]["url"]
        with self.assertRaises(build.BuildError):
            build.validate(data)

    def test_empty_changes_fails(self):
        data = minimal_data()
        data["changes"] = []
        with self.assertRaises(build.BuildError):
            build.validate(data)

    def test_change_missing_blocks_fails(self):
        data = minimal_data()
        data["changes"][0]["blocks"] = []
        with self.assertRaises(build.BuildError):
            build.validate(data)

    def test_missing_review_fails(self):
        data = minimal_data()
        del data["review"]
        with self.assertRaises(build.BuildError):
            build.validate(data)


class TestBuild(unittest.TestCase):
    def test_full_build_escapes_and_fills_placeholders(self):
        data = minimal_data()
        data["pr"]["title"] = "Fix <a> & <b>"
        data["changes"][0]["blocks"] = [{"lang": "go", "code": "if a < b {}"}]
        out = build.build(data)
        # No placeholder survives.
        for token in ("{{PR_TITLE}}", "{{PR_URL}}", "{{PR_FILES_URL}}",
                      "{{PR_NUMBER}}", "{{REPO}}", "{{RISK_SCORE}}",
                      "{{CONTENT}}"):
            self.assertNotIn(token, out)
        # Title and code are escaped in the output.
        self.assertIn("Fix &lt;a&gt; &amp; &lt;b&gt;", out)
        self.assertIn("if a &lt; b {}", out)

    def test_topbar_risk_badge_carries_score(self):
        data = minimal_data()
        data["risk"]["score"] = 8
        out = build.build(data)
        # The topbar badge and its score span both get the filled score.
        self.assertIn('id="topbar-risk" href="#risk" data-score="8"', out)
        self.assertIn('<span id="topbar-risk-score">8</span>', out)

    def test_block_risk_chip_and_lines_render(self):
        data = minimal_data()
        data["changes"][0]["blocks"] = [
            {"lang": "go", "code": "close := true",
             "risk": {"score": 7, "reason": "terminal flag",
                      "lines": [{"match": "close", "severity": "high",
                                 "note": "wrong value stalls stream"}]}}]
        out = build.build(data)
        self.assertIn('class="block-risk-chip" data-score="7"', out)
        self.assertIn("data-risk=", out)
        self.assertIn("wrong value stalls stream", out)

    def test_dual_views_render_each_change(self):
        data = minimal_data()
        data["changes"][0]["title"] = "UniqueChangeTitle"
        out = build.build(data)
        # Rendered in both the canvas node and the list article.
        self.assertIn(
            '<span class="cnode-title">UniqueChangeTitle</span>', out)
        self.assertIn(
            '<h3 class="font-semibold text-brand-soft mb-1">'
            'UniqueChangeTitle</h3>', out)

    def test_shell_matches_docs_brand_and_dark_visual_language(self):
        out = build.build(minimal_data())
        self.assertIn("--wt-primary: #6366f1", out)
        self.assertIn("--wt-bg: #121212", out)
        self.assertIn("--wt-surface: #191919", out)
        self.assertIn("--wt-dark-surface: #1e1e1e", out)
        self.assertIn("--wt-fg: #ebebeb", out)
        self.assertIn("font-family: 'Figtree'", out)
        self.assertIn("font-family: 'Geist Mono'", out)
        self.assertIn('class="wt-topbar sticky', out)
        self.assertIn('class="wt-brand-mark"', out)
        self.assertIn('href="https://kandev.ai/favicon.ico"', out)
        self.assertIn('href="https://kandev.ai/icon.svg"', out)
        self.assertIn(
            'rel="apple-touch-icon" href="https://kandev.ai/apple-touch-icon.png"',
            out,
        )
        self.assertIn('src="https://kandev.ai/brand/kandev-github-org.png"', out)
        self.assertIn(
            "brand: { DEFAULT: '#6366f1', soft: '#818cf8', deep: '#4f46e5' },",
            out,
        )

    def test_runtime_cdn_dependencies_use_exact_versions(self):
        out = build.build(minimal_data())
        for url in (
            "https://cdn.tailwindcss.com/3.4.17",
            "https://cdn.jsdelivr.net/npm/mermaid@11.17.0/dist/mermaid.esm.min.mjs",
            "https://cdn.jsdelivr.net/npm/marked@12.0.2/+esm",
            "https://cdn.jsdelivr.net/npm/dompurify@3.4.14/+esm",
            "https://cdn.jsdelivr.net/npm/shiki@3.23.0/+esm",
        ):
            self.assertIn(url, out)
        self.assertNotIn('src="https://cdn.tailwindcss.com"', out)

    def test_shell_has_mobile_section_navigation(self):
        out = build.build(minimal_data())
        self.assertIn('id="mobile-nav"', out)
        self.assertIn('id="mobile-nav-architecture"', out)
        self.assertIn('id="mobile-nav-data"', out)
        self.assertIn("@media (max-width: 640px)", out)

    def test_shell_makes_code_scrollbars_subtle_until_hover(self):
        out = build.build(minimal_data())
        self.assertIn("scrollbar-width: thin", out)
        self.assertIn("scrollbar-color: transparent transparent", out)
        self.assertIn(".shiki::-webkit-scrollbar-thumb", out)
        self.assertIn(".shiki:hover::-webkit-scrollbar-thumb", out)
        self.assertIn(".shiki:focus-within::-webkit-scrollbar-thumb", out)

    def test_shell_uses_lanes_to_route_canvas_edges(self):
        out = build.build(minimal_data())
        self.assertIn("function laneOffset", out)
        self.assertIn("e.dataset.fromLane", out)
        self.assertIn("e.dataset.toLane", out)
        self.assertIn("e.dataset.pairLane", out)
        self.assertIn("const labelLane", out)
        self.assertIn("const labelFromLane", out)
        self.assertIn("const labelToLane", out)
        self.assertIn("const endpointLabelLane", out)
        self.assertIn("const pairLabelLane", out)
        self.assertIn("labelPoint.x += labelLane", out)
        self.assertIn("sizeGroups(); drawEdges(); fit();", out)


if __name__ == "__main__":
    unittest.main()
