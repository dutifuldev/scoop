import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { CollectionDropdown } from "./CollectionDropdown";
import shellStyles from "../../styles/shell.css?raw";

function renderCollectionDropdown(): void {
  render(
    <CollectionDropdown
      selectedCollection="ai-news"
      allCollectionsValue="__all__"
      allCollectionsLabel="All collections (3)"
      currentCollectionLabel="AI News"
      collections={[
        {
          collection: "ai-news",
          translation_mode: "disabled",
          articles: 10,
          stories: 2,
          story_items: 7,
        },
        {
          collection: "world-news",
          translation_mode: "disabled",
          articles: 20,
          stories: 1,
          story_items: 4,
        },
      ]}
      onCollectionChange={vi.fn()}
    />,
  );
}

describe("CollectionDropdown", () => {
  it("renders title and collection in single-line classes", () => {
    const { container } = render(
      <CollectionDropdown
        selectedCollection="ai-news"
        allCollectionsValue="__all__"
        allCollectionsLabel="All collections (3)"
        currentCollectionLabel="AI News"
        collections={[
          {
            collection: "ai-news",
            translation_mode: "disabled",
            articles: 10,
            stories: 2,
            story_items: 7,
          },
        ]}
        onCollectionChange={vi.fn()}
      />,
    );

    expect(screen.getByText("SCOOP")).toHaveClass("brand-select-prefix");
    expect(screen.getByText("AI News")).toHaveClass("brand-select-current");
    expect(
      container.querySelector("button.brand-select-trigger > div.brand-select-label"),
    ).toBeInTheDocument();
    expect(container.querySelector(".brand-select-separator-dot")).toBeInTheDocument();
    expect(shellStyles).toContain(".brand-select-label");
    expect(shellStyles).toContain("whitespace-nowrap");
  });

  it("opens and closes the dropdown list", async () => {
    const user = userEvent.setup();
    renderCollectionDropdown();

    const trigger = screen.getByLabelText("Collection filter: AI News");
    await user.click(trigger);

    expect(await screen.findByRole("option", { name: "world-news (1)" })).toBeInTheDocument();

    await user.keyboard("{Escape}");
    await waitFor(() => {
      expect(screen.queryByRole("option", { name: "world-news (1)" })).not.toBeInTheDocument();
    });
  });

  it("keeps explicit hover/open selectors in shell styles", () => {
    expect(shellStyles).toContain(".brand-select-trigger:hover");
    expect(shellStyles).toContain('.brand-select-trigger[data-state="open"]');
  });
});
