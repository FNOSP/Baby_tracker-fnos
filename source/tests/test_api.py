import os
import tempfile
import unittest
from datetime import date, timedelta

import app as app_module
from werkzeug.security import generate_password_hash


def _generate_password_hash(password):
    # macOS system Python 3.9 may lack hashlib.scrypt; the app runs on Python 3.11.
    return generate_password_hash(password, method='pbkdf2:sha256')


class ApiTestCase(unittest.TestCase):
    def setUp(self):
        app_module.generate_password_hash = _generate_password_hash
        self.temp_dir = tempfile.TemporaryDirectory()
        app_module.app.config.update(
            DATABASE=os.path.join(self.temp_dir.name, 'test.db'),
            TESTING=True,
        )
        app_module._db_initialized = False
        app_module._ha_button_states.clear()
        for timer in app_module._ha_button_timers.values():
            timer.cancel()
        app_module._ha_button_timers.clear()
        self.client = app_module.app.test_client()
        self.client.get('/api/auth/me')

    def tearDown(self):
        for timer in app_module._ha_button_timers.values():
            timer.cancel()
        app_module._ha_button_timers.clear()
        app_module._ha_button_states.clear()
        app_module._db_initialized = False
        self.temp_dir.cleanup()

    def _insert_record(self, record_type, sub_type, amount, timestamp):
        with app_module.app.app_context():
            db = app_module.get_db()
            cursor = db.execute(
                """
                INSERT INTO records (baby_id, user_id, type, sub_type, amount, timestamp)
                VALUES (1, 1, ?, ?, ?, ?)
                """,
                (record_type, sub_type, amount, timestamp),
            )
            db.commit()
            return cursor.lastrowid

    def test_today_summary_uses_latest_feed_across_days(self):
        today = date.today().isoformat()
        yesterday = (date.today() - timedelta(days=1)).isoformat()

        self._insert_record('feed', 'breast_left', 70, f'{yesterday} 23:30:00')

        response = self.client.get(f'/api/records/today?date={today}')
        self.assertEqual(response.status_code, 200)
        payload = response.get_json()
        self.assertEqual(payload['total_feed_ml'], 0)
        self.assertEqual(payload['last_feed_time'], f'{yesterday} 23:30:00')

    def test_latest_feed_uses_id_as_tie_breaker(self):
        today = date.today().isoformat()
        timestamp = f'{today} 08:00:00'

        self._insert_record('feed', 'formula', 60, timestamp)
        latest_id = self._insert_record('feed', 'formula', 80, timestamp)

        response = self.client.get(f'/api/records/today?date={today}')
        self.assertEqual(response.status_code, 200)
        self.assertEqual(response.get_json()['last_feed_time'], timestamp)
        self.assertEqual(response.get_json()['total_feed_ml'], 140)

        with app_module.app.app_context():
            latest = app_module._get_latest_feed(app_module.get_db())
            self.assertEqual(latest['id'], latest_id)

    def test_latest_feed_includes_solid_food(self):
        today = date.today().isoformat()
        self._insert_record('feed', 'formula', 100, f'{today} 08:00:00')
        self._insert_record('feed', 'solid_food', 30, f'{today} 09:00:00')

        response = self.client.get(f'/api/records/today?date={today}')
        self.assertEqual(response.status_code, 200)
        self.assertEqual(response.get_json()['last_feed_time'], f'{today} 09:00:00')

    def test_ha_status_exposes_standard_sensor_payload(self):
        today = date.today().isoformat()
        yesterday = (date.today() - timedelta(days=1)).isoformat()

        self._insert_record('feed', 'formula', 120, f'{today} 08:00:00')
        self._insert_record('excrete', 'urine', None, f'{today} 09:00:00')
        self._insert_record('excrete', 'both', None, f'{today} 10:00:00')
        self._insert_record('feed', 'breast_right', 90, f'{yesterday} 23:00:00')

        response = self.client.get(f'/api/ha/status?date={today}')
        self.assertEqual(response.status_code, 200)
        payload = response.get_json()

        self.assertEqual(payload['state'], 120)
        self.assertEqual(payload['attributes']['total_feed_ml'], 120)
        self.assertEqual(payload['attributes']['feed_count'], 1)
        self.assertEqual(payload['attributes']['urine_count'], 2)
        self.assertEqual(payload['attributes']['stool_count'], 1)
        self.assertEqual(payload['attributes']['last_feed_time'], f'{today} 08:00:00')

        last_feed = self.client.get('/api/ha/last-feed').get_json()
        self.assertEqual(last_feed['state'], f'{today} 08:00:00')
        self.assertEqual(last_feed['attributes']['amount_ml'], 120)

    def test_ha_button_accepts_bearer_and_x_api_key_headers(self):
        with app_module.app.app_context():
            db = app_module.get_db()
            button_id = db.execute(
                "SELECT id FROM quick_buttons WHERE is_active = 1 ORDER BY sort_order LIMIT 1"
            ).fetchone()['id']
            db.execute(
                "INSERT OR REPLACE INTO settings (key, value) VALUES ('ha_api_key', 'test-key')"
            )
            db.commit()

        response = self.client.post(
            f'/api/ha/button/{button_id}',
            headers={'Authorization': 'Bearer test-key'},
            json={'state': 'on'},
        )
        self.assertEqual(response.status_code, 200)
        self.assertEqual(response.get_json()['state'], 'on')

        response = self.client.post(
            f'/api/ha/button/{button_id}',
            headers={'X-API-Key': 'test-key'},
            json={'state': 'on'},
        )
        self.assertEqual(response.status_code, 200)

        response = self.client.post(f'/api/ha/button/{button_id}', json={'state': 'on'})
        self.assertEqual(response.status_code, 401)

    def test_default_vaccine_schedule_matches_2026(self):
        def ages(short):
            return [
                item['age_months']
                for item in app_module.VACCINE_SCHEDULE
                if item['short'] == short
            ]

        self.assertEqual(app_module.VACCINE_SCHEDULE_VERSION, '2026年版')
        self.assertEqual(len(app_module.VACCINE_SCHEDULE), 30)
        self.assertEqual(ages('MMR'), [8, 24])
        self.assertEqual(ages('JE-I'), [8, 8, 24, 156])
        self.assertEqual(ages('2vHPV'), [156, 156])
        self.assertFalse(
            any('2025新规' in item['note'] for item in app_module.VACCINE_SCHEDULE)
        )

        page = self.client.get('/vaccine')
        self.assertEqual(page.status_code, 200)
        self.assertIn('2026年版', page.get_data(as_text=True))

    def test_vaccine_schedule_api_returns_2026_dates(self):
        birth = date.today() - timedelta(days=365)
        with app_module.app.app_context():
            db = app_module.get_db()
            db.execute("DELETE FROM babies")
            db.execute(
                "INSERT INTO babies (name, gender, birth_date, weight) VALUES (?, ?, ?, ?)",
                ('宝宝', 'male', birth.isoformat(), 3.0),
            )
            db.commit()

        response = self.client.get('/api/vaccine/schedule')
        self.assertEqual(response.status_code, 200)
        schedule = response.get_json()['schedule']
        mmr_second = next(
            item for item in schedule
            if item['short'] == 'MMR' and item['dose_index'] == 2
        )
        expected_due = (birth + timedelta(days=int(24 * 30.44))).isoformat()
        self.assertEqual(mmr_second['due_date'], expected_due)
        self.assertEqual(
            [item['dose_index'] for item in schedule if item['short'] == '2vHPV'],
            [1, 2],
        )


if __name__ == '__main__':
    unittest.main()
