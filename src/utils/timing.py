# src/utils/timing.py
import functools
import time
from typing import Any, Callable


def retry(
    retries: int = 3,
    delay: float = 0.5,
    backoff=False,
    backoff_ratio=2,
    exceptions: tuple = (Exception,),
    logger_func: Callable[[str], None] = print,
):
    """
    A simple retry decorator with delay and optional logger.
    """

    def decorator(func: Callable) -> Callable:
        @functools.wraps(func)
        def wrapper(*args, **kwargs) -> Any:
            current_delay = delay
            for attempt in range(1, retries + 1):
                try:
                    return func(*args, **kwargs)
                except exceptions as e:
                    if attempt == retries:
                        raise
                    logger_func(f"[retry] Attempt {attempt} failed: {e}. Retrying in {delay}s...")
                    time.sleep(current_delay)
                    if backoff:
                        current_delay *= backoff_ratio

        return wrapper

    return decorator
