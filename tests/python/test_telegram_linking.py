"""Telegram pairing and per-chat state. There are no accounts and no API keys, so
the one-time link code is the whole authorisation story: it must expire, work
once, and never leak the bot token back to a plain profile read."""

import datetime as dt

from core import profile
from schemas.profile import PROFILE_ID, LinkCode


def test_a_fresh_install_has_an_empty_profile(clean_db):
    p = profile.get_profile()
    assert p.id == PROFILE_ID
    assert not p.onboarded
    assert not p.telegram_enabled
    assert not p.telegram_bot_token


def test_saving_the_name_completes_onboarding(clean_db):
    saved = profile.save_name("  Khan  ")
    assert saved.name == "Khan"
    assert saved.onboarded is True


def test_the_bot_token_lives_in_the_database_not_the_environment(clean_db):
    profile.save_telegram_settings(bot_token="123:secret-token", enabled=True, bot_username="shamsu_bot")

    assert profile.telegram_config() == {"bot_token": "123:secret-token", "enabled": True}

    public = profile.public_profile()
    assert public["telegram"]["configured"] is True
    assert public["telegram"]["bot_username"] == "shamsu_bot"
    assert "123:secret-token" not in repr(public), "the raw token must not reach the public profile"


def test_a_link_code_authorises_one_chat_exactly_once(clean_db):
    profile.save_telegram_settings(bot_token="t", enabled=True, bot_username="shamsu_bot")
    issued = profile.new_link_code()

    assert issued["deep_link"].endswith("?start=" + issued["code"])
    assert issued["expires_in"] == 600

    assert profile.verify_link_code(issued["code"], chat_id=555, tg_username="khan") is True
    assert profile.get_link(555).authorized is True

    # A used code cannot authorise a second chat.
    assert profile.verify_link_code(issued["code"], chat_id=666) is False
    assert profile.get_link(666) is None


def test_an_unknown_or_expired_code_is_refused(clean_db):
    from sqlmodel import Session

    from core.database import get_engine

    assert profile.verify_link_code("deadbeef", chat_id=1) is False

    stale = "expired1"
    with Session(get_engine()) as session:
        # profile.py stores naive UTC, so match that shape without the deprecated call.
        past = dt.datetime.now(dt.UTC).replace(tzinfo=None) - dt.timedelta(minutes=1)
        session.add(LinkCode(code=stale, expires_at=past))
        session.commit()

    assert profile.verify_link_code(stale, chat_id=1) is False
    assert profile.get_link(1) is None


def test_an_unlinked_chat_starts_unauthorised(clean_db):
    link = profile.ensure_link(777, tg_username="stranger", display_name="Stranger")

    assert link.authorized is False, "merely messaging the bot must not grant access"
    assert profile.get_link(777).tg_username == "stranger"


def test_each_chat_keeps_its_own_project_session_and_mode(clean_db):
    for chat in (10, 20):
        profile.ensure_link(chat)

    profile.set_selected_project(10, "project-a", "session-a")
    profile.set_selected_project(20, "project-b", "session-b")
    profile.set_mode(10, "auto")
    profile.set_mode(20, "plan")
    profile.set_session(10, "session-a2")

    a, b = profile.get_link(10), profile.get_link(20)
    assert (a.selected_project_id, a.active_session_id, a.mode) == ("project-a", "session-a2", "auto")
    assert (b.selected_project_id, b.active_session_id, b.mode) == ("project-b", "session-b", "plan")


def test_revoking_a_link_removes_it_from_the_profile(clean_db):
    code = profile.new_link_code()["code"]
    profile.verify_link_code(code, chat_id=999, display_name="Phone")
    assert any(l["chat_id"] == 999 for l in profile.list_links())

    profile.revoke_link(999)

    assert profile.get_link(999) is None
    assert profile.list_links() == []


def test_state_setters_ignore_an_unlinked_chat(clean_db):
    profile.set_mode(4242, "auto")
    profile.set_session(4242, "s")
    profile.set_selected_project(4242, "p", "s")
    assert profile.get_link(4242) is None
