#!/usr/bin/env python3
"""
LemonHarness Web Search — Academic & general web search for agentic research.

Uses:
- DuckDuckGo Search API for general web results (no API key needed)
- arXiv OAI-PMH API for academic paper searches
- Semantic Scholar API for paper metadata and citations

Usage:
  python3 search.py --web "query"          # General web search
  python3 search.py --arxiv "query"        # arXiv paper search
  python3 search.py --semantic "query"     # Semantic Scholar search
  python3 search.py --recent "query"       # Recent (2026) papers on topic

All results include URLs and dates for proper citation.
"""

import sys
import json
import re
from datetime import datetime, timezone
from xml.etree import ElementTree

try:
    from ddgs import DDGS
except ImportError:
    DDGS = None

try:
    import requests
except ImportError:
    requests = None


def search_web(query: str, max_results: int = 8) -> list[dict]:
    """General web search via DuckDuckGo. No API key needed."""
    if DDGS is None:
        return [{"title": "Error", "url": "", "snippet": "DuckDuckGo search library not available. Run: python3 -m venv /tmp/search-env && /tmp/search-env/bin/pip install duckduckgo_search"}]

    results = []
    try:
        with DDGS() as ddgs:
            for i, r in enumerate(ddgs.text(query, max_results=max_results)):
                results.append({
                    "title": r.get("title", ""),
                    "url": r.get("href", r.get("link", "")),
                    "snippet": r.get("body", ""),
                    "source": "web",
                })
    except Exception as e:
        results.append({"title": f"Search error: {e}", "url": "", "snippet": "", "source": "error"})

    return results


def search_arxiv(query: str, max_results: int = 8, year_from: int | None = None) -> list[dict]:
    """Search arXiv via OAI-PMH API. Free, no API key needed."""
    if requests is None:
        return [{"title": "Error", "url": "", "snippet": "requests library not available"}]

    # Build query
    # arXiv uses a prefix-based query syntax
    # We want recent papers, sorted by relevance
    clean_query = re.sub(r'[^\w\s-]', ' ', query).strip()
    # Add year filter if specified
    if year_from:
        # arXiv API date filtering is complex; we'll filter post-query instead
        pass

    params = {
        "search_query": f"all:{clean_query}",
        "start": 0,
        "max_results": max_results,
        "sortBy": "relevance",
        "sortOrder": "descending",
    }

    try:
        resp = requests.get(
            "https://export.arxiv.org/api/query",
            params=params,
            timeout=15,
            headers={"User-Agent": "LemonHarness/1.0 (research-citation-tool)"},
        )
        resp.raise_for_status()
    except Exception as e:
        return [{"title": f"arXiv API error: {e}", "url": "", "snippet": "", "source": "error"}]

    # Parse Atom XML response
    ns = {
        "atom": "http://www.w3.org/2005/Atom",
        "arxiv": "http://arxiv.org/schemas/atom",
    }

    results = []
    try:
        root = ElementTree.fromstring(resp.content)
        for entry in root.findall("atom:entry", ns):
            title = entry.findtext("atom:title", "").strip().replace("\n", " ")
            summary = entry.findtext("atom:summary", "").strip().replace("\n", " ")[:500]

            # Get arXiv ID from the link
            paper_id = ""
            for link in entry.findall("atom:link", ns):
                if link.get("rel") == "alternate":
                    href = link.get("href", "")
                    paper_id = href.split("/abs/")[-1] if "/abs/" in href else href
                    break

            # Get published date
            published = entry.findtext("atom:published", "", ns)
            updated = entry.findtext("atom:updated", "", ns)

            # Get authors
            authors = []
            for author in entry.findall("atom:author", ns):
                name = author.findtext("atom:name", "", ns)
                if name:
                    authors.append(name)

            # Get categories
            categories = []
            for cat in entry.findall("atom:category", ns):
                term = cat.get("term", "")
                if term:
                    categories.append(term)

            # arXiv URL
            url = f"https://arxiv.org/abs/{paper_id}" if paper_id else ""

            paper_data = {
                "title": title,
                "url": url,
                "snippet": summary[:300],
                "authors": authors[:5],
                "published": published[:10] if published else "",
                "updated": updated[:10] if updated else "",
                "categories": categories[:3],
                "paper_id": paper_id,
                "source": "arxiv",
                "year": int(published[:4]) if published and published[:4].isdigit() else 0,
            }

            # Filter by year if specified
            if year_from and paper_data["year"] < year_from:
                continue

            results.append(paper_data)

    except Exception as e:
        return [{"title": f"arXiv parse error: {e}", "url": "", "snippet": str(resp.content)[:200], "source": "error"}]

    # If no results from arXiv, try a fallback with a simpler query
    if not results:
        # Try without special characters
        simple_query = " ".join(clean_query.split()[:5])
        if simple_query != clean_query:
            return search_arxiv(simple_query, max_results, year_from)

    return results


def search_semantic_scholar(query: str, max_results: int = 8, year_from: int | None = None) -> list[dict]:
    """Search Semantic Scholar API for academic papers."""
    if requests is None:
        return [{"title": "Error", "url": "", "snippet": "requests library not available"}]

    params = {
        "query": query,
        "limit": max_results,
        "fields": "title,url,abstract,authors,year,venue,citationCount,publicationDate,externalIds",
    }
    if year_from:
        params["year"] = f"{year_from}-"

    try:
        resp = requests.get(
            "https://api.semanticscholar.org/graph/v1/paper/search",
            params=params,
            timeout=15,
            headers={"User-Agent": "LemonHarness/1.0"},
        )
        resp.raise_for_status()
        data = resp.json()
    except Exception as e:
        return [{"title": f"Semantic Scholar error: {e}", "url": "", "snippet": "", "source": "error"}]

    results = []
    for paper in data.get("data", []):
        authors = [a.get("name", "") for a in paper.get("authors", [])[:5]]
        published = (paper.get("publicationDate") or str(paper.get("year", "")) or "")
        paper_id = paper.get("paperId", "")
        ext_ids = paper.get("externalIds", {})

        results.append({
            "title": paper.get("title", ""),
            "url": paper.get("url", "") or f"https://www.semanticscholar.org/paper/{paper_id}",
            "snippet": (paper.get("abstract") or "")[:300],
            "authors": authors,
            "published": str(published)[:10],
            "year": paper.get("year", 0),
            "venue": paper.get("venue", ""),
            "citationCount": paper.get("citationCount", 0),
            "arxiv_id": ext_ids.get("ArXiv", ""),
            "source": "semantic-scholar",
        })

    return results


def format_results(results: list[dict], source: str) -> str:
    """Format search results as a readable string with proper citations."""
    if not results:
        return f"No results found from {source}."

    if results[0].get("source") == "error":
        return f"⚠ Search error: {results[0]['title']}"

    lines = [f"📚 Search Results ({source}):", ""]

    for i, r in enumerate(results, 1):
        lines.append(f"  [{i}] {r.get('title', 'Untitled')}")

        if r.get("authors"):
            authors = r["authors"]
            author_str = ", ".join(authors[:3])
            if len(authors) > 3:
                author_str += " et al."
            lines.append(f"      Authors: {author_str}")

        if r.get("published"):
            lines.append(f"      Date: {r['published']}")

        if r.get("year"):
            lines.append(f"      Year: {r['year']}")

        if r.get("venue"):
            lines.append(f"      Venue: {r['venue']}")

        if r.get("citationCount") is not None and r["citationCount"] > 0:
            lines.append(f"      Citations: {r['citationCount']}")

        if r.get("categories"):
            lines.append(f"      Categories: {', '.join(r['categories'])}")

        if r.get("arxiv_id"):
            lines.append(f"      arXiv: {r['arxiv_id']}")

        if r.get("url"):
            lines.append(f"      URL: {r['url']}")

        if r.get("snippet"):
            snippet = r["snippet"][:200]
            lines.append(f"      {snippet}")

        lines.append("")

    return "\n".join(lines)


def main():
    if len(sys.argv) < 3:
        print(__doc__)
        sys.exit(1)

    mode = sys.argv[1]
    
    # Parse --max-results flag
    max_results = 8
    query_parts = []
    i = 2
    while i < len(sys.argv):
        if sys.argv[i] == "--max-results" and i + 1 < len(sys.argv):
            try:
                max_results = int(sys.argv[i + 1])
                i += 2
            except ValueError:
                i += 1
        else:
            query_parts.append(sys.argv[i])
            i += 1
    query = " ".join(query_parts)

    if not query.strip():
        print("Please provide a search query.")
        sys.exit(1)

    if mode == "--web":
        results = search_web(query, max_results=max_results)
        print(format_results(results, "web"))

    elif mode == "--arxiv":
        results = search_arxiv(query, max_results=max_results)
        print(format_results(results, "arXiv"))

    elif mode == "--semantic":
        # Search for recent (2026) papers
        year = datetime.now(timezone.utc).year
        results_arxiv = search_arxiv(query, max_results=max_results, year_from=year - 1)
        results_semantic = search_semantic_scholar(query, max_results=max_results, year_from=year - 1)

        output = []
        if results_arxiv and results_arxiv[0].get("source") != "error":
            output.append(format_results(results_arxiv, f"arXiv (recent, {year})"))
            output.append("")
        if results_semantic and results_semantic[0].get("source") != "error":
            output.append(format_results(results_semantic, f"Semantic Scholar (recent, {year})"))

        if not output:
            # Fallback to general web search
            results = search_web(f"{query} 2026")
            output.append(format_results(results, f"web (recent, {year})"))

        print("\n".join(output))

    elif mode == "--json":
        # Return JSON for programmatic use
        if query.startswith("arxiv:"):
            results = search_arxiv(query[6:])
        elif query.startswith("semantic:"):
            results = search_semantic_scholar(query[9:])
        else:
            results = search_web(query)
        print(json.dumps(results, indent=2))

    else:
        print(f"Unknown mode: {mode}")
        print(__doc__)
        sys.exit(1)


if __name__ == "__main__":
    main()
