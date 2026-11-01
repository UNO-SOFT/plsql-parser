BEGIN
  IF p_flag = 'END IF BEGIN' THEN
    LOOP
      EXIT;
    END LOOP;
  END IF;
  CASE p_id WHEN 1 THEN NULL; END CASE;
  v_name := CASE WHEN 1 = 1 THEN 'BEGIN' ELSE 'END' END;
END;
END package_spec;
END package_body;
