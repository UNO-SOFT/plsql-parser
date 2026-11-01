BEGIN
  IF ready THEN
    write_data('alma');
    update_log('x');
    done;
  END IF;
  tail_a;

  IF ready THEN
    write_data('körte');
    update_log('y');
    done;
  END IF;
  tail_b;

  /* IF ready THEN
    write_data('dead');
    update_log('z');
    done;
  END IF; */

  IF ready THEN
    write_data('barack');
    update_log('z');
    done;
  END IF;
  tail_c;
END;
