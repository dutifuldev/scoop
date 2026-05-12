#!/usr/bin/env python3
"""
Metal News Fetcher — Gathers nonferrous metals news from CNIA (cnmn.com.cn), Reddit,
and Western English-language metals/mining news RSS feeds.

Usage:
    python3 fetch_news.py [--hours 48] [--max 200] [--format json]

Sources:
    - cnmn.com.cn (中国有色网 — CNIA official media outlet)
    - Reddit r/Copper, r/aluminum, r/mining, r/commodities
    - Metal Miner (agmetalminer.com/feed/) — industrial metals industry news
    - Mining Technology (mining-technology.com/feed/) — global mining news
"""

import argparse
import hashlib
import json
import re
import sys
import urllib.request
import urllib.error
import xml.etree.ElementTree as ET
from datetime import datetime, timedelta, timezone
from email.utils import parsedate_to_datetime
from html import unescape
from concurrent.futures import ThreadPoolExecutor, as_completed


# --- CNIA Sections ---

CNMN_SECTIONS = {
    "要闻 (Headlines)": "https://www.cnmn.com.cn/ShowNewsList.aspx?id=13",
    "铜 (Copper)": "https://www.cnmn.com.cn/metal.aspx?id=1",
    "铝 (Aluminum)": "https://www.cnmn.com.cn/metal.aspx?id=23",
    "铅锌 (Lead/Zinc)": "https://www.cnmn.com.cn/metal.aspx?id=35",
    "镍钴 (Nickel/Cobalt)": "https://www.cnmn.com.cn/metal.aspx?id=14",
    "锡锑 (Tin/Antimony)": "https://www.cnmn.com.cn/metal.aspx?id=22",
    "贵金属 (Precious)": "https://www.cnmn.com.cn/metal.aspx?id=87",
    "市场行情 (Market)": "https://www.cnmn.com.cn/NewsMarket.aspx",
}

REDDIT_SUBREDDITS = [
    "Copper",
    "aluminum",
    "mining",
    "commodities",
]

METAL_KEYWORDS = [
    "copper", "aluminum", "aluminium", "zinc", "lead", "nickel",
    "cobalt", "tin", "antimony", "tungsten", "molybdenum", "titanium",
    "rare earth", "lithium", "gold", "silver", "platinum", "palladium",
    "smelter", "smelting", "refinery", "mining", "ore", "concentrate",
    "lme", "shfe", "comex", "metal price", "metal market",
    "nonferrous", "non-ferrous", "base metal",
    "铜", "铝", "铅", "锌", "镍", "钴", "锡", "锑", "钨",
    "钼", "钛", "稀土", "锂", "贵金属", "有色金属", "冶炼",
]


def fetch_url(url, timeout=15):
    try:
        req = urllib.request.Request(url, headers={
            "User-Agent": "Mozilla/5.0 (compatible; MetalNewsBot/1.0)"
        })
        return urllib.request.urlopen(req, timeout=timeout).read()
    except Exception as e:
        print(f"  [warn] fetch failed {url}: {e}", file=sys.stderr)
        return None


def make_item_id(source, unique_part):
    h = hashlib.md5(f"{source}:{unique_part}".encode()).hexdigest()[:16]
    return h


def parse_cnmn_date(text, now=None):
    """Parse CNMN article/listing dates from text. Returns UTC datetime or None."""
    now = now or datetime.now(timezone.utc)
    if not text:
        return None

    # Full article pages usually include full year and time.
    m = re.search(r'(20\d{2})年\s*(\d{1,2})月\s*(\d{1,2})日(?:\s+(\d{1,2}):(\d{2}))?', text)
    if m:
        year, month, day = map(int, m.group(1, 2, 3))
        hour = int(m.group(4) or 0)
        minute = int(m.group(5) or 0)
        return datetime(year, month, day, hour, minute, tzinfo=timezone.utc)

    # Section/listing snippets often only include month/day, e.g. "4月16日，..."
    m = re.search(r'(\d{1,2})月\s*(\d{1,2})日', text)
    if m:
        month, day = map(int, m.group(1, 2))
        year = now.year
        # If month appears to be from late last year while we're early in a new year, roll back.
        if month > now.month + 1:
            year -= 1
        return datetime(year, month, day, tzinfo=timezone.utc)

    return None


# --- CNIA Scraper ---

def scrape_cnmn_section(section_name, url):
    """Scrape article links + titles from a CNIA section page."""
    data = fetch_url(url)
    if not data:
        return []
    text = data.decode("utf-8", errors="replace")
    pattern = r'href="(/ShowNews1\.aspx\?id=(\d+))"[^>]*>\s*([^<]{3,})'
    items = []
    seen_ids = set()

    for match in re.finditer(pattern, text):
        href, article_id, title = match.groups()
        title = unescape(title.strip().replace("&nbsp;", " "))
        if not title or article_id in seen_ids:
            continue
        seen_ids.add(article_id)
        full_url = f"https://www.cnmn.com.cn{href}"

        # Only trust dates visible in the section/listing page.
        # If we cannot parse a date there, skip the item rather than invent freshness.
        nearby = text[match.end(): match.end() + 600]
        published_dt = parse_cnmn_date(nearby)
        if published_dt is None:
            continue

        items.append({
            "source": "cnmn.com.cn",
            "source_item_id": make_item_id("cnmn", article_id),
            "title": title,
            "url": full_url,
            "published_at": published_dt.isoformat(),
            "section": section_name,
        })
    return items


def fetch_cnmn(hours=48, max_per_section=30):
    """Fetch recent articles from all CNIA sections in parallel."""
    now = datetime.now(timezone.utc)
    cutoff = now - timedelta(hours=hours)
    all_items = []
    seen_ids = set()
    with ThreadPoolExecutor(max_workers=6) as pool:
        futures = {
            pool.submit(scrape_cnmn_section, name, url): name
            for name, url in CNMN_SECTIONS.items()
        }
        for future in as_completed(futures):
            name = futures[future]
            try:
                items = future.result()
                kept = 0
                for item in items:
                    published_dt = datetime.fromisoformat(item["published_at"])
                    if published_dt < cutoff:
                        continue
                    if published_dt > now + timedelta(days=1):
                        continue
                    if item["source_item_id"] in seen_ids:
                        continue
                    seen_ids.add(item["source_item_id"])
                    all_items.append(item)
                    kept += 1
                    if kept >= max_per_section:
                        break
                print(f"  [cnmn] {name}: {kept} recent articles", file=sys.stderr)
            except Exception as e:
                print(f"  [warn] {name} failed: {e}", file=sys.stderr)
    return all_items


# --- Western RSS Feeds ---

WESTERN_RSS_FEEDS = [
    {
        "name": "Metal Miner",
        "url": "https://agmetalminer.com/feed/",
        "source": "agmetalminer.com",
        "no_keyword_filter": True,  # all content is metals-focused
    },
    {
        "name": "Mining Technology",
        "url": "https://www.mining-technology.com/feed/",
        "source": "mining-technology.com",
        "no_keyword_filter": True,  # all content is mining-focused
    },
]


def parse_rss_date(date_str):
    """Parse RSS pubDate (RFC 2822 or ISO 8601) to UTC datetime, or return None."""
    if not date_str:
        return None
    date_str = date_str.strip()
    # Try RFC 2822 (standard RSS)
    try:
        return parsedate_to_datetime(date_str).astimezone(timezone.utc)
    except Exception:
        pass
    # Try ISO 8601
    for fmt in ("%Y-%m-%dT%H:%M:%S%z", "%Y-%m-%dT%H:%M:%SZ", "%Y-%m-%d %H:%M:%S"):
        try:
            dt = datetime.strptime(date_str, fmt)
            if dt.tzinfo is None:
                dt = dt.replace(tzinfo=timezone.utc)
            return dt.astimezone(timezone.utc)
        except ValueError:
            pass
    return None


def _find_el(item, *tags):
    """Find first matching element by trying multiple tag names. Avoids bool(Element) bug."""
    for tag in tags:
        el = item.find(tag)
        if el is not None:
            return el
    return None


def fetch_rss(feed, hours=48, max_items=30):
    """Fetch and parse an RSS feed, returning story dicts."""
    cutoff = datetime.now(timezone.utc) - timedelta(hours=hours)
    data = fetch_url(feed["url"])
    if not data:
        return []

    try:
        root = ET.fromstring(data)
    except ET.ParseError as e:
        print(f"  [warn] RSS parse error {feed['url']}: {e}", file=sys.stderr)
        return []

    # Handle both <rss> and <feed> (Atom) roots
    ns_atom = "http://www.w3.org/2005/Atom"
    ns_dc = "http://purl.org/dc/elements/1.1/"
    items_xml = root.findall(".//item")
    if not items_xml:
        items_xml = root.findall(f".//{{{ns_atom}}}entry")

    no_filter = feed.get("no_keyword_filter", False)
    results = []
    for item in items_xml:
        # Title
        title_el = _find_el(item, "title", f"{{{ns_atom}}}title")
        title = unescape((title_el.text or "").strip()) if title_el is not None else ""
        if not title or len(title) < 5:
            continue

        # URL
        link_el = _find_el(item, "link", f"{{{ns_atom}}}link")
        if link_el is not None:
            url = (link_el.text or link_el.get("href", "")).strip()
        else:
            url = ""

        # Published date — try multiple tag names
        pub_el = _find_el(
            item,
            "pubDate",
            "published",
            f"{{{ns_atom}}}published",
            f"{{{ns_dc}}}date",
        )
        pub_str = (pub_el.text or "").strip() if pub_el is not None else ""
        pub_dt = parse_rss_date(pub_str)
        if pub_dt is None:
            continue
        if pub_dt < cutoff:
            continue

        # Filter by metal keywords (skip for inherently metals-focused feeds)
        if not no_filter:
            title_lower = title.lower()
            desc_el = _find_el(item, "description", f"{{{ns_atom}}}summary")
            desc = unescape((desc_el.text or "").strip()) if desc_el is not None else ""
            desc_lower = desc.lower()
            if not any(kw in title_lower or kw in desc_lower for kw in METAL_KEYWORDS):
                continue

        item_id = make_item_id(feed["source"], url or title)
        results.append({
            "source": feed["source"],
            "source_item_id": item_id,
            "title": title,
            "url": url,
            "published_at": pub_dt.isoformat(),
            "feed_name": feed["name"],
        })
        if len(results) >= max_items:
            break

    print(f"  [{feed['name']}] {len(results)} items", file=sys.stderr)
    return results


def fetch_western_rss(hours=48):
    """Fetch all western RSS feeds in parallel."""
    all_items = []
    seen_ids = set()
    with ThreadPoolExecutor(max_workers=4) as pool:
        futures = {
            pool.submit(fetch_rss, feed, hours): feed["name"]
            for feed in WESTERN_RSS_FEEDS
        }
        for future in as_completed(futures):
            name = futures[future]
            try:
                items = future.result()
                for item in items:
                    if item["source_item_id"] not in seen_ids:
                        seen_ids.add(item["source_item_id"])
                        all_items.append(item)
            except Exception as e:
                print(f"  [warn] {name} failed: {e}", file=sys.stderr)
    return all_items


# --- Reddit Scraper ---

def fetch_reddit(subreddits, hours=48, max_per_sub=25):
    cutoff = datetime.now(timezone.utc) - timedelta(hours=hours)
    all_items = []
    for sub in subreddits:
        url = f"https://www.reddit.com/r/{sub}/new.json?limit=50"
        data = fetch_url(url)
        if not data:
            continue
        try:
            listing = json.loads(data)
            posts = listing.get("data", {}).get("children", [])
        except (json.JSONDecodeError, KeyError):
            continue
        count = 0
        for post in posts:
            d = post.get("data", {})
            created = datetime.fromtimestamp(d.get("created_utc", 0), tz=timezone.utc)
            if created < cutoff:
                continue
            title = d.get("title", "").strip()
            if not title:
                continue
            post_url = d.get("url", "")
            permalink = f"https://www.reddit.com{d.get('permalink', '')}"
            all_items.append({
                "source": "reddit",
                "source_item_id": make_item_id("reddit", d.get("id", "")),
                "title": title,
                "url": post_url if post_url and not post_url.startswith("https://www.reddit.com") else permalink,
                "published_at": created.isoformat(),
                "score": d.get("score", 0),
                "num_comments": d.get("num_comments", 0),
                "subreddit": sub,
            })
            count += 1
            if count >= max_per_sub:
                break
        print(f"  [reddit] r/{sub}: {count} posts", file=sys.stderr)
    return all_items


# --- Main ---

def main():
    parser = argparse.ArgumentParser(description="Fetch metal news from CNIA and Reddit")
    parser.add_argument("--hours", type=int, default=48, help="Lookback window for Reddit (default: 48)")
    parser.add_argument("--max", type=int, default=200, help="Max total items (default: 200)")
    parser.add_argument("--format", choices=["json", "markdown"], default="json", help="Output format")
    args = parser.parse_args()

    print(f"Fetching metal news (lookback: {args.hours}h)...", file=sys.stderr)

    # Fetch all sources
    cnmn_items = fetch_cnmn(hours=args.hours)
    reddit_items = fetch_reddit(REDDIT_SUBREDDITS, hours=args.hours)
    western_items = fetch_western_rss(hours=args.hours)

    all_items = cnmn_items + reddit_items + western_items

    # Deduplicate by source_item_id
    seen = set()
    unique = []
    for item in all_items:
        sid = item["source_item_id"]
        if sid not in seen:
            seen.add(sid)
            unique.append(item)

    # Trim to max
    unique = unique[: args.max]

    print(
        f"Total: {len(unique)} items "
        f"({len(cnmn_items)} CNIA, {len(reddit_items)} Reddit, {len(western_items)} Western RSS)",
        file=sys.stderr,
    )

    if args.format == "json":
        output = {
            "collection": "metal_news",
            "fetched_at": datetime.now(timezone.utc).isoformat(),
            "item_count": len(unique),
            "items": unique,
        }
        json.dump(output, sys.stdout, ensure_ascii=False, indent=2)
        sys.stdout.write("\n")
    else:
        print(f"# Metal News Digest ({datetime.now(timezone.utc).strftime('%Y-%m-%d')})\n")
        print(f"**{len(unique)} stories** from CNIA, Reddit, Metal Miner, Mining Technology\n")
        for item in unique:
            src = item["source"]
            title = item["title"]
            url = item["url"]
            section = item.get("section", "")
            if src == "cnmn.com.cn":
                prefix = f"🇨🇳 [{section}]"
            elif item.get("feed_name"):
                prefix = f"🌐 [{item.get('feed_name', src)}]"
            else:
                prefix = "💬"
            print(f"- {prefix} [{title}]({url})")


if __name__ == "__main__":
    main()
