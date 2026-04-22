from __future__ import annotations

import math
import re
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
RESULTS_FILE = ROOT / "EVALUATION_RESULTS.md"
OUTPUT_DIR = ROOT / "presentation" / "graphs"


def parse_markdown_table(lines: list[str], title: str) -> tuple[list[str], list[list[str]]]:
    for index, line in enumerate(lines):
        if line.strip() != title:
            continue

        header_index = index + 1
        while header_index < len(lines) and not lines[header_index].strip().startswith("|"):
            header_index += 1

        separator_index = header_index + 1
        while separator_index < len(lines) and not lines[separator_index].strip().startswith("|"):
            separator_index += 1

        header = [cell.strip() for cell in lines[header_index].strip().strip("|").split("|")]
        rows: list[list[str]] = []
        cursor = separator_index + 1
        while cursor < len(lines):
            current = lines[cursor].strip()
            if not current.startswith("|"):
                break
            rows.append([cell.strip() for cell in current.strip("|").split("|")])
            cursor += 1
        return header, rows

    raise ValueError(f"Could not find table {title!r} in {RESULTS_FILE}")


def parse_results() -> dict[str, object]:
    lines = RESULTS_FILE.read_text().splitlines()

    _, latency_rows = parse_markdown_table(lines, "### Latency")
    _, throughput_rows = parse_markdown_table(lines, "### Throughput")
    _, availability_rows = parse_markdown_table(lines, "### Availability")
    _, security_rows = parse_markdown_table(lines, "### Security And Moderation")
    _, cross_region_rows = parse_markdown_table(lines, "### Cross-Region Convergence")
    _, partition_rows = parse_markdown_table(lines, "### Partition-Heal Convergence")

    latency: dict[str, dict[str, float]] = {}
    for row in latency_rows:
        latency[row[0]] = {
            "count": float(row[1]),
            "min_ms": float(row[2]),
            "avg_ms": float(row[3]),
            "p50_ms": float(row[4]),
            "p95_ms": float(row[5]),
            "p99_ms": float(row[6]),
            "max_ms": float(row[7]),
        }

    throughput = {row[0]: row[1] for row in throughput_rows}
    availability = {row[0]: row[1:] for row in availability_rows}
    security = {row[0]: row[1] for row in security_rows}
    cross_region = {row[0]: row[1] for row in cross_region_rows}
    partition = {row[0]: row[1] for row in partition_rows}

    return {
        "latency": latency,
        "throughput": throughput,
        "availability": availability,
        "security": security,
        "cross_region": cross_region,
        "partition": partition,
    }


def svg_text(x: float, y: float, text: str, size: int = 14, weight: str = "400", fill: str = "#1f2937", anchor: str = "start") -> str:
    return (
        f'<text x="{x}" y="{y}" font-family="system-ui, -apple-system, sans-serif" '
        f'font-size="{size}" font-weight="{weight}" fill="{fill}" text-anchor="{anchor}">{text}</text>'
    )


def svg_rect(x: float, y: float, width: float, height: float, fill: str, rx: int = 0) -> str:
    return f'<rect x="{x}" y="{y}" width="{width}" height="{height}" rx="{rx}" fill="{fill}" />'


def svg_line(x1: float, y1: float, x2: float, y2: float, stroke: str = "#e5e7eb", width: int = 1) -> str:
    return f'<line x1="{x1}" y1="{y1}" x2="{x2}" y2="{y2}" stroke="{stroke}" stroke-width="{width}" />'


def svg_circle(cx: float, cy: float, radius: float, fill: str) -> str:
    return f'<circle cx="{cx}" cy="{cy}" r="{radius}" fill="{fill}" />'


def save_svg(name: str, width: int, height: int, body: list[str]) -> None:
    OUTPUT_DIR.mkdir(parents=True, exist_ok=True)
    svg = [
        f'<svg xmlns="http://www.w3.org/2000/svg" width="{width}" height="{height}" viewBox="0 0 {width} {height}">',
        '<rect width="100%" height="100%" fill="#ffffff" />',
        *body,
        '</svg>',
    ]
    (OUTPUT_DIR / name).write_text("\n".join(svg))


def graph_latency_summary(results: dict[str, object]) -> None:
    latency = results["latency"]
    labels = ["Baseline\nread", "Baseline\nwrite", "Failover\nread", "Failover\nwrite", "Churn\nread", "Churn\nwrite"]
    colors = ["#3b82f6", "#06b6d4", "#8b5cf6", "#14b8a6", "#f59e0b", "#ef4444"]
    keys = ["Baseline read latency", "Baseline write latency", "Failover read latency", 
            "Failover write latency", "Churn read latency", "Churn write latency"]
    values = [latency[key]["avg_ms"] for key in keys]
    max_log = math.log10(max(values) + 1)

    width = 1000
    height = 600
    left = 80
    right = 40
    chart_top = 100
    chart_bottom = 500
    chart_height = chart_bottom - chart_top
    bar_width = 100
    gap = 35

    body = [
        svg_text(left, 50, "Average Latency by Scenario", size=24, weight="600", fill="#111827"),
        svg_line(left, chart_bottom, left + (bar_width + gap) * 6 - gap + 20, chart_bottom, stroke="#d1d5db", width=2),
    ]

    # Y-axis ticks
    for tick in [0, 1, 2, 3, 4, 5]:
        value = 10**tick if tick > 0 else 1
        y = chart_bottom - (tick / max_log) * chart_height if max_log else chart_bottom
        body.append(svg_line(left - 5, y, left, y, stroke="#9ca3af", width=2))
        label = "1" if tick == 0 else f"10^{tick}"
        body.append(svg_text(left - 15, y + 5, label, size=12, fill="#6b7280", anchor="end"))

    # Bars
    for index, (label, value, color) in enumerate(zip(labels, values, colors)):
        bar_height = (math.log10(value + 1) / max_log) * chart_height if value > 0 else 0
        x = left + index * (bar_width + gap) + 10
        y = chart_bottom - bar_height
        
        body.append(svg_rect(x, y, bar_width, bar_height, color, rx=3))
        body.append(svg_text(x + bar_width/2, y - 8, f"{value:.1f}", size=12, weight="600", fill=color, anchor="middle"))
        
        # X-axis labels
        for line_num, line in enumerate(label.split("\n")):
            body.append(svg_text(x + bar_width/2, chart_bottom + 25 + line_num * 18, line, size=13, fill="#374151", anchor="middle"))

    save_svg("latency_summary.svg", width, height, body)


def graph_tail_latency(results: dict[str, object]) -> None:
    latency = results["latency"]
    series = [
        ("Churn\nread", latency["Churn read latency"]),
        ("Churn\nwrite", latency["Churn write latency"]),
        ("Failover\nwrite", latency["Failover write latency"]),
    ]
    metrics = [("p95_ms", "P95", "#3b82f6"), ("p99_ms", "P99", "#f59e0b"), ("max_ms", "Max", "#ef4444")]

    width = 1000
    height = 600
    left = 100
    right = 60
    chart_top = 100
    chart_bottom = 500
    chart_height = chart_bottom - chart_top
    chart_width = width - left - right
    
    # Use log scale for better visibility
    all_values = [row[key] for _, row in series for key, _, _ in metrics if row[key] > 0]
    max_value = max(all_values)
    max_log = math.log10(max_value + 1)

    body = [
        svg_text(left, 50, "Tail Latency Under Failure (Log Scale)", size=24, weight="600", fill="#111827"),
        svg_text(left, 75, "Logarithmic scale shows all bars proportionally", size=12, fill="#6b7280"),
        svg_line(left, chart_bottom, width - right, chart_bottom, stroke="#d1d5db", width=2),
    ]

    # Y-axis with logarithmic ticks
    log_ticks = [1, 10, 100, 1000, 10000, 100000]
    for tick_value in log_ticks:
        if tick_value <= max_value:
            log_val = math.log10(tick_value + 1)
            ratio = log_val / max_log
            y = chart_bottom - ratio * chart_height
            body.append(svg_line(left - 5, y, left, y, stroke="#9ca3af", width=2))
            if tick_value >= 1000:
                label = f"{int(tick_value/1000)}k"
            else:
                label = str(tick_value)
            body.append(svg_text(left - 15, y + 5, label, size=12, fill="#6b7280", anchor="end"))

    # Bars with log scale
    group_width = chart_width / len(series)
    bar_width = 45
    for group_index, (label, row) in enumerate(series):
        group_x = left + group_index * group_width + 40
        
        for metric_index, (key, _, color) in enumerate(metrics):
            value = row[key]
            if value > 0:
                log_height = math.log10(value + 1) / max_log
                height_px = log_height * chart_height
            else:
                height_px = 0
            x = group_x + metric_index * (bar_width + 15)
            y = chart_bottom - height_px
            
            body.append(svg_rect(x, y, bar_width, height_px, color, rx=2))
            
            # Value labels above bars
            if height_px > 20:
                if value >= 1000:
                    label_text = f"{int(value/1000)}k"
                else:
                    label_text = str(int(value))
                body.append(svg_text(x + bar_width/2, y - 5, label_text, size=10, weight="600", fill=color, anchor="middle"))
        
        # Group label
        for line_num, line in enumerate(label.split("\n")):
            body.append(svg_text(group_x + 80, chart_bottom + 25 + line_num * 18, line, size=13, fill="#374151", anchor="middle"))

    # Legend
    legend_x = width - 180
    for index, (_, metric_label, color) in enumerate(metrics):
        y = 105 + index * 24
        body.append(svg_rect(legend_x, y - 10, 16, 16, color, rx=2))
        body.append(svg_text(legend_x + 24, y + 3, metric_label, size=13, fill="#374151"))

    save_svg("tail_latency.svg", width, height, body)


def graph_throughput_availability(results: dict[str, object]) -> None:
    throughput = results["throughput"]
    availability = results["availability"]
    
    ops_per_second = float(throughput["Operations per second"])
    total_ops = int(throughput["Total mixed-workload operations"])
    successful = int(throughput["Successful operations"])
    failed = int(throughput["Failed operations"])
    duration = float(throughput["Elapsed time"].strip(" s"))
    
    # Parse availability with counts
    failover_read_frac = availability["Failover availability"][0]
    failover_write_frac = availability["Failover availability"][2]
    churn_read_frac = availability["Churn availability"][0]
    churn_write_frac = availability["Churn availability"][2]

    width = 1000
    height = 600
    
    body = [
        svg_text(80, 50, "System Performance Under Failure", size=24, weight="600", fill="#111827"),
    ]

    # Three metric cards
    cards = [
        ("Throughput", f"{ops_per_second:.0f}", "ops/sec", "#3b82f6", 80),
        ("Total Ops", f"{total_ops}", f"{successful} succeeded", "#10b981", 380),
        ("Duration", f"{duration:.1f}s", f"{failed} failed", "#6b7280", 680),
    ]
    
    for title, main_val, sub_val, color, x in cards:
        body.append(svg_rect(x, 90, 260, 140, "#f9fafb", rx=4))
        body.append(svg_text(x + 20, 120, title, size=14, weight="600", fill="#6b7280"))
        body.append(svg_text(x + 20, 170, main_val, size=40, weight="700", fill=color))
        body.append(svg_text(x + 20, 205, sub_val, size=13, fill="#6b7280"))

    # Availability with actual counts
    body.append(svg_text(80, 280, "Request Completion During Failures", size=18, weight="600", fill="#111827"))
    
    scenarios = [
        ("Failover - Read Requests", failover_read_frac, "#3b82f6"),
        ("Failover - Write Requests", failover_write_frac, "#06b6d4"),
        ("Churn - Read Requests", churn_read_frac, "#10b981"),
        ("Churn - Write Requests", churn_write_frac, "#059669"),
    ]
    
    top = 320
    row_height = 50
    gap = 15
    
    for index, (label, fraction, color) in enumerate(scenarios):
        y = top + index * (row_height + gap)
        parts = fraction.split("/")
        succeeded = parts[0]
        total = parts[1]
        percent = (int(succeeded) / int(total)) * 100 if int(total) > 0 else 0
        
        body.append(svg_text(80, y + 17, label, size=14, fill="#374151"))
        body.append(svg_text(80, y + 38, f"{fraction} completed", size=11, fill="#6b7280"))
        
        # Progress bar
        bar_x = 380
        bar_width = 440
        body.append(svg_rect(bar_x, y + 5, bar_width, 35, "#f3f4f6", rx=2))
        body.append(svg_rect(bar_x, y + 5, bar_width * (percent / 100.0), 35, color, rx=2))
        body.append(svg_text(bar_x + bar_width + 20, y + 26, f"{percent:.0f}%", size=16, weight="600", fill=color))

    save_svg("throughput_availability.svg", width, height, body)


def metric_value(metric_map: dict[str, str], key: str) -> float:
    raw = metric_map[key]
    match = re.search(r"([0-9]+(?:\.[0-9]+)?)", raw)
    if not match:
        raise ValueError(f"Could not parse numeric value from {key!r}: {raw!r}")
    return float(match.group(1))


def graph_convergence_and_security(results: dict[str, object]) -> None:
    cross_region = results["cross_region"]
    partition = results["partition"]
    security = results["security"]

    cross_value = metric_value(cross_region, "Cross-region convergence time")
    cross_threshold = metric_value(cross_region, "Threshold")
    partition_value = metric_value(partition, "Partition-heal convergence time")
    partition_threshold = metric_value(partition, "Threshold")
    malicious_posts = int(metric_value(security, "Malicious posts created by Mallory"))
    visible_after_pruning = int(metric_value(security, "Mallory-visible posts after pruning on follower"))
    ban_code = int(metric_value(security, "Mallory post-after-ban HTTP code"))

    width = 1000
    height = 600
    
    body = [
        svg_text(80, 50, "Convergence and Moderation", size=24, weight="600", fill="#111827"),
    ]

    # Convergence metrics
    cards = [
        ("Cross-Region Convergence", cross_value, cross_threshold, "#3b82f6", 80),
        ("Partition-Heal Convergence", partition_value, partition_threshold, "#10b981", 540),
    ]
    
    for title, value, threshold, color, x in cards:
        y = 100
        body.append(svg_rect(x, y, 380, 180, "#f9fafb", rx=4))
        body.append(svg_text(x + 20, y + 35, title, size=16, weight="600", fill="#374151"))
        body.append(svg_text(x + 20, y + 100, f"{int(value)} ms", size=42, weight="700", fill=color))
        body.append(svg_text(x + 20, y + 135, f"Threshold: {int(threshold)} ms", size=13, fill="#6b7280"))
        body.append(svg_text(x + 20, y + 160, f"Headroom: {int(threshold - value)} ms", size=13, fill="#059669"))

    # Moderation metrics
    mod_y = 330
    body.append(svg_rect(80, mod_y, 840, 200, "#f9fafb", rx=4))
    body.append(svg_text(100, mod_y + 35, "Moderation Effectiveness", size=16, weight="600", fill="#374151"))
    
    body.append(svg_text(100, mod_y + 80, f"Malicious posts created: {malicious_posts}", size=14, fill="#6b7280"))
    body.append(svg_text(100, mod_y + 110, f"Visible after pruning: {visible_after_pruning}", size=14, fill="#6b7280"))
    body.append(svg_text(100, mod_y + 140, f"Post-ban response code: {ban_code}", size=14, fill="#6b7280"))
    
    # Simple flow
    flow_x = 600
    body.append(svg_circle(flow_x, mod_y + 100, 30, "#f59e0b"))
    body.append(svg_text(flow_x, mod_y + 108, str(malicious_posts), size=20, weight="700", fill="#ffffff", anchor="middle"))
    body.append(svg_line(flow_x + 40, mod_y + 100, flow_x + 100, mod_y + 100, stroke="#d1d5db", width=2))
    body.append(svg_circle(flow_x + 140, mod_y + 100, 30, "#10b981"))
    body.append(svg_text(flow_x + 140, mod_y + 108, str(visible_after_pruning), size=20, weight="700", fill="#ffffff", anchor="middle"))

    save_svg("convergence_security.svg", width, height, body)


def main() -> None:
    results = parse_results()
    graph_latency_summary(results)
    graph_tail_latency(results)
    graph_throughput_availability(results)
    graph_convergence_and_security(results)
    print(f"Wrote graphs to {OUTPUT_DIR}")


if __name__ == "__main__":
    main()