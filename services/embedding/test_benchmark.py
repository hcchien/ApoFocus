import unittest

from benchmark import backend_decision, percentile, summarize


class BenchmarkReportTests(unittest.TestCase):
    def test_percentile_uses_nearest_rank(self):
        self.assertEqual(percentile([10, 20, 30, 40], 0.5), 20)
        self.assertEqual(percentile([10, 20, 30, 40], 0.95), 40)

    def test_summary_keeps_stage_breakdown_and_failures(self):
        report = summarize([
            {"wallMs": 100, "timingsMs": {"thumbnailMs": 20, "totalMs": 90}},
            {"wallMs": 200, "timingsMs": {"thumbnailMs": 30, "totalMs": 180}},
            {"error": "broken"},
        ])
        self.assertEqual(report["succeeded"], 2)
        self.assertEqual(report["failed"], 1)
        self.assertEqual(report["stagesMs"]["thumbnailMs"]["total"], 50)

    def test_backend_switch_requires_threshold_and_enough_samples(self):
        records = [
            {"wallMs": 100, "timingsMs": {"thumbnailMs": 25, "totalMs": 100}}
            for _ in range(20)
        ]
        self.assertEqual(backend_decision(records, 20)["decision"], "evaluate_libvips")
        self.assertEqual(backend_decision(records[:10], 20)["decision"], "keep_pillow")


if __name__ == "__main__":
    unittest.main()
