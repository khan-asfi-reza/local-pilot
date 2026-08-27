"""The in-process event bus. Every full-access run publishes here so a passive
watcher (the web Code IDE open on a project a Telegram chat is editing) can
render the run live."""

import asyncio

from services import run_bus


def drain(queue) -> list:
    out = []
    while not queue.empty():
        out.append(queue.get_nowait())
    return out


def test_events_reach_every_watcher_of_a_project(project_dir):
    async def scenario():
        a = run_bus.subscribe(project_dir)
        b = run_bus.subscribe(project_dir)
        run_bus.publish(project_dir, {"type": "text", "content": "hi"})
        run_bus.publish(project_dir, {"type": "done"})
        return drain(a), drain(b)

    first, second = asyncio.run(scenario())
    assert first == second
    assert [e["type"] for e in first] == ["text", "done"]


def test_watchers_only_see_their_own_project(tmp_path):
    one = str(tmp_path / "one")
    two = str(tmp_path / "two")
    (tmp_path / "one").mkdir()
    (tmp_path / "two").mkdir()

    async def scenario():
        q = run_bus.subscribe(one)
        run_bus.publish(two, {"type": "text", "content": "other project"})
        return drain(q)

    assert asyncio.run(scenario()) == []


def test_the_same_project_by_a_different_path_is_the_same_channel(project_dir):
    """Paths are normalised, so ./project and /abs/project are one channel."""
    async def scenario():
        q = run_bus.subscribe(project_dir)
        run_bus.publish(project_dir + "/.", {"type": "text", "content": "same"})
        return drain(q)

    assert len(asyncio.run(scenario())) == 1


def test_publishing_with_no_watchers_is_a_no_op(project_dir):
    run_bus.publish(project_dir, {"type": "text", "content": "nobody listening"})


def test_a_slow_watcher_never_blocks_the_run(project_dir):
    """A watcher whose queue fills up drops events; the agent run must not stall."""
    async def scenario():
        q = run_bus.subscribe(project_dir)
        for i in range(1200):  # the queue holds 1000
            run_bus.publish(project_dir, {"type": "text", "content": str(i)})
        return q.qsize()

    assert asyncio.run(scenario()) == 1000


def test_unsubscribing_stops_delivery(project_dir):
    async def scenario():
        q = run_bus.subscribe(project_dir)
        run_bus.unsubscribe(project_dir, q)
        run_bus.publish(project_dir, {"type": "text", "content": "after"})
        run_bus.unsubscribe(project_dir, q)  # unsubscribing twice is harmless
        return drain(q)

    assert asyncio.run(scenario()) == []
